package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type amapConfig struct{ WebKey, JSKey, SecurityCode, DefaultCity string }

// Only this module reads .env; it never loads credentials into the process environment.
// Reading per request lets the local owner update credentials without restarting.
func loadAMapConfig() (amapConfig, error) {
	values := map[string]string{}
	path := os.Getenv("AMAP_CONFIG_FILE")
	if path == "" {
		path = ".env"
	}
	file, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return amapConfig{}, errors.New("地图配置暂时无法读取")
	}
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(io.LimitReader(file, 64<<10))
		for scanner.Scan() {
			line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
			if !ok {
				continue
			}
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
				quote := value[:1]
				closing := strings.Index(value[1:], quote)
				if closing < 0 {
					return amapConfig{}, errors.New("地图配置格式有误")
				}
				value = value[1 : closing+1]
			} else if index := strings.Index(value, " #"); index >= 0 {
				value = strings.TrimSpace(value[:index])
			}
			values[key] = value
		}
		if scanner.Err() != nil {
			return amapConfig{}, errors.New("地图配置格式有误")
		}
	}
	get := func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return strings.TrimSpace(value)
		}
		return values[key]
	}
	return amapConfig{get("AMAP_WEB_KEY"), get("AMAP_JS_KEY"), get("AMAP_SECURITY_JS_CODE"), get("AMAP_DEFAULT_CITY")}, nil
}

type locationResult struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Address           string `json:"address"`
	Province          string `json:"province"`
	City              string `json:"city"`
	District          string `json:"district"`
	Latitude          string `json:"latitude"`
	ConfirmationToken string `json:"confirmation_token"`
	Longitude         string `json:"longitude"`
	CoordinateSystem  string `json:"coordinate_system"`
	Source            string `json:"source"`
}

type locationHandler struct {
	config     func() (amapConfig, error)
	client     *http.Client
	limitsOnce sync.Once
	limits     *mapRequestLimiter
}

func registerLocationRoutes(router *gin.Engine) {
	h := &locationHandler{config: loadAMapConfig, client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	router.GET("/api/maps/config", h.configuration)
	router.POST("/api/locations/search", h.withMapLimits(h.search))
	router.POST("/api/locations/reverse", h.withMapLimits(h.reverse))
	router.GET("/_AMapService/*path", h.proxy)
}

func (h *locationHandler) configuration(c *gin.Context) {
	config, err := h.config()
	c.Header("Cache-Control", "no-store")
	if err != nil {
		c.JSON(200, gin.H{"map_enabled": false, "search_enabled": false, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"map_enabled": config.JSKey != "" && config.SecurityCode != "", "search_enabled": config.WebKey != "", "js_key": config.JSKey, "security_proxy": "/_AMapService", "default_city": config.DefaultCity})
}

func (h *locationHandler) serviceConfig(c *gin.Context) (amapConfig, bool) {
	config, err := h.config()
	if err != nil || config.WebKey == "" {
		c.JSON(503, gin.H{"error": "地址搜索尚未连接，请配置后重试。"})
		return config, false
	}
	return config, true
}

func (h *locationHandler) amapJSON(ctx context.Context, path string, params url.Values, config amapConfig) (map[string]interface{}, error) {
	params.Set("key", config.WebKey)
	params.Set("output", "JSON")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://restapi.amap.com"+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, errors.New("无法查询地图服务")
	}
	response, err := h.client.Do(req)
	if err != nil {
		return nil, errors.New("地图服务连接超时或暂时不可用，请重试。")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("地图服务暂时不可用，请稍后重试。")
	}
	var result map[string]interface{}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result) != nil {
		return nil, errors.New("地图服务返回了无法读取的结果。")
	}
	if result["status"] != "1" {
		if textField(result, "infocode") == "10009" {
			return nil, errors.New("高德地址服务 Key 类型不匹配（10009），请使用 Web 服务类型的 Key。")
		}
		return nil, errors.New("地图查询失败，请检查高德服务权限、配置或调用额度。")
	}
	return result, nil
}

func textField(data map[string]interface{}, key string) string {
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

func parseLocation(value string) (string, string, bool) {
	pair := strings.Split(value, ",")
	if len(pair) != 2 {
		return "", "", false
	}
	lon, errLon := strconv.ParseFloat(strings.TrimSpace(pair[0]), 64)
	lat, errLat := strconv.ParseFloat(strings.TrimSpace(pair[1]), 64)
	if errLon != nil || errLat != nil || math.IsNaN(lon) || math.IsNaN(lat) || math.IsInf(lon, 0) || math.IsInf(lat, 0) || math.Abs(lon) > 180 || math.Abs(lat) > 90 {
		return "", "", false
	}
	return strconv.FormatFloat(lon, 'f', 6, 64), strconv.FormatFloat(lat, 'f', 6, 64), true
}

func fullAddress(province, city, district, address string) string {
	prefix := province
	if city != province {
		prefix += city
	}
	prefix += district
	if strings.HasPrefix(address, prefix) || province != "" && strings.HasPrefix(address, province) {
		return address
	}
	if city != "" && strings.HasPrefix(address, city) {
		return province + address
	}
	if district != "" && strings.HasPrefix(address, district) {
		return strings.TrimSuffix(prefix, district) + address
	}
	return prefix + address
}

func (h *locationHandler) search(c *gin.Context) {
	var input struct {
		Keyword string `json:"keyword"`
		City    string `json:"city"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "请输入有效的地址搜索条件。"})
		return
	}
	keyword, city := strings.TrimSpace(input.Keyword), strings.TrimSpace(input.City)
	if keyword == "" || utf8.RuneCountInString(keyword) > 80 || utf8.RuneCountInString(city) > 40 {
		c.JSON(400, gin.H{"error": "请输入 1–80 字的地点名称或详细地址。"})
		return
	}
	config, ok := h.serviceConfig(c)
	if !ok {
		return
	}
	if city == "" {
		city = config.DefaultCity
	}
	params := url.Values{"keywords": {keyword}, "page_size": {"8"}, "page_num": {"1"}}
	if city != "" {
		params.Set("region", city)
		params.Set("city_limit", "true")
	}
	data, err := h.amapJSON(c.Request.Context(), "/v5/place/text", params, config)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	results := []locationResult{}
	pois, _ := data["pois"].([]interface{})
	for _, value := range pois {
		poi, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		lon, lat, ok := parseLocation(textField(poi, "location"))
		if !ok {
			continue
		}
		province, city, district := textField(poi, "pname"), textField(poi, "cityname"), textField(poi, "adname")
		name := textField(poi, "name")
		address := textField(poi, "address")
		if address == "" {
			address = name
		}
		if name == "" {
			continue
		}
		results = append(results, locationResult{ID: textField(poi, "id"), Name: name, Address: fullAddress(province, city, district, address), Province: province, City: city, District: district, Longitude: lon, Latitude: lat, CoordinateSystem: "GCJ-02", Source: "amap"})
	}
	if len(results) == 0 {
		geocodes, geoErr := h.amapJSON(c.Request.Context(), "/v3/geocode/geo", url.Values{"address": {keyword}, "city": {city}}, config)
		if geoErr != nil {
			c.JSON(502, gin.H{"error": geoErr.Error()})
			return
		}
		items, _ := geocodes["geocodes"].([]interface{})
		for _, value := range items {
			item, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			lon, lat, ok := parseLocation(textField(item, "location"))
			if !ok {
				continue
			}
			address := textField(item, "formatted_address")
			if address == "" {
				continue
			}
			results = append(results, locationResult{Name: keyword, Address: address, Province: textField(item, "province"), City: textField(item, "city"), District: textField(item, "district"), Longitude: lon, Latitude: lat, CoordinateSystem: "GCJ-02", Source: "amap"})
		}
	}
	for index := range results {
		results[index] = signLocation(results[index])
	}
	c.JSON(200, gin.H{"locations": results})
}

func (h *locationHandler) reverse(c *gin.Context) {
	var input struct {
		Longitude string `json:"longitude"`
		Latitude  string `json:"latitude"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "请选择有效的地图位置。"})
		return
	}
	lon, lat, valid := parseLocation(input.Longitude + "," + input.Latitude)
	if !valid {
		c.JSON(400, gin.H{"error": "请选择有效的地图位置。"})
		return
	}
	config, ok := h.serviceConfig(c)
	if !ok {
		return
	}
	data, err := h.amapJSON(c.Request.Context(), "/v3/geocode/regeo", url.Values{"location": {lon + "," + lat}, "extensions": {"base"}, "radius": {"100"}}, config)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	item, _ := data["regeocode"].(map[string]interface{})
	address := textField(item, "formatted_address")
	if address == "" {
		c.JSON(404, gin.H{"error": "此位置未找到完整地址，请选择附近建筑或通过地址搜索。"})
		return
	}
	component, _ := item["addressComponent"].(map[string]interface{})
	province, city := textField(component, "province"), textField(component, "city")
	if city == "" && (province == "北京市" || province == "上海市" || province == "天津市" || province == "重庆市") {
		city = province
	}
	c.JSON(200, gin.H{"location": signLocation(locationResult{Name: "地图选点", Address: address, Province: province, City: city, District: textField(component, "district"), Longitude: lon, Latitude: lat, CoordinateSystem: "GCJ-02", Source: "amap"})})
}

// This is a fixed-origin, read-only SDK proxy. Clients cannot choose an upstream host
// or inject either credential; the JS safety code never reaches the browser.
var mapCallbackPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)*$`)

func (h *locationHandler) proxy(c *gin.Context) {
	release, ok := h.acquireMapRequest(c)
	if !ok {
		return
	}
	defer release()
	if len(c.Request.URL.RawQuery) > 16<<10 {
		c.Status(http.StatusBadRequest)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	for _, name := range []string{"callback", "cb", "jsonp"} {
		for _, value := range query[name] {
			if len(value) > 128 || !mapCallbackPattern.MatchString(value) {
				c.Status(http.StatusBadRequest)
				return
			}
		}
	}
	config, err := h.config()
	if err != nil || config.JSKey == "" || config.SecurityCode == "" {
		c.JSON(503, gin.H{"error": "高德地图尚未连接"})
		return
	}
	path := c.Param("path")
	allowed := false
	for _, prefix := range []string{"/v4/maps", "/v3/vectormap", "/v4/map/styles", "/v3/place/", "/v5/place/", "/v3/geocode/", "/v3/config/district"} {
		if path == prefix || strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix) {
			allowed = true
			break
		}
	}
	if !allowed || strings.Contains(path, "..") || strings.ContainsAny(path, "?#%\\") {
		c.JSON(404, gin.H{"error": "地图请求不存在"})
		return
	}
	origin := "https://restapi.amap.com"
	if path == "/v4/map/styles" {
		origin = "https://webapi.amap.com"
	}
	query.Set("key", config.JSKey)
	query.Set("jscode", config.SecurityCode)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+path+"?"+query.Encode(), nil)
	if err != nil {
		c.Status(502)
		return
	}
	client := *h.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		c.JSON(502, gin.H{"error": "地图服务暂时不可用"})
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		c.JSON(502, gin.H{"error": "地图服务暂时不可用"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		c.Status(502)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, response.Header.Get("Content-Type"), body)
}
