package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type amapTestTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport amapTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = transport.target.Scheme
	clone.URL.Host = transport.target.Host
	return transport.base.RoundTrip(clone)
}

func testAMapHandler(t *testing.T, upstream http.HandlerFunc) *locationHandler {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	return &locationHandler{config: func() (amapConfig, error) {
		return amapConfig{WebKey: "test-web-key", JSKey: "test-js-key", SecurityCode: "test-private-code"}, nil
	}, client: &http.Client{Transport: amapTestTransport{target: target, base: http.DefaultTransport}}}
}
func callLocation(handler gin.HandlerFunc, path string) *httptest.ResponseRecorder {
	router := gin.New()
	router.POST("/", handler)
	recorder := httptest.NewRecorder()
	parsed, _ := url.Parse(path)
	input := map[string]string{}
	for key, values := range parsed.Query() {
		input[key] = values[0]
	}
	body, _ := json.Marshal(input)
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/", strings.NewReader(string(body))))
	return recorder
}

func TestAMapSearchNormalizesCompleteAddressAndCoordinates(t *testing.T) {
	var keyword, region, key string
	h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) {
		keyword = r.URL.Query().Get("keywords")
		region = r.URL.Query().Get("region")
		key = r.URL.Query().Get("key")
		io.WriteString(w, `{"status":"1","pois":[{"id":"one","name":"员岗地铁站","pname":"广东省","cityname":"广州市","adname":"番禺区","address":"兴南大道468号","location":"113.368476,23.013742"},{"name":"缺少坐标","location":[]},{"name":"非法位置","location":"NaN,23"}]}`)
	})
	response := callLocation(h.search, "/?keyword=%E5%91%98%E5%B2%97&city=%E5%B9%BF%E5%B7%9E%E5%B8%82")
	if response.Code != 200 {
		t.Fatalf("search failed: %s", response.Body.String())
	}
	var result struct {
		Locations []locationResult `json:"locations"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	if len(result.Locations) != 1 {
		t.Fatalf("invalid coordinate candidates were retained: %+v", result.Locations)
	}
	place := result.Locations[0]
	if !verifyLocation(SignRequest{Address: place.Address, AddressName: place.Name, Latitude: place.Latitude, Longitude: place.Longitude, ConfirmationToken: place.ConfirmationToken}) {
		t.Fatal("search did not sign exact candidate fields")
	}
	if place.Address != "广东省广州市番禺区兴南大道468号" || place.Longitude != "113.368476" || place.Latitude != "23.013742" || place.CoordinateSystem != "GCJ-02" {
		t.Fatalf("inconsistent location: %+v", place)
	}
	if keyword != "员岗" || region != "广州市" || key != "test-web-key" {
		t.Fatalf("incorrect provider query: %q %q %q", keyword, region, key)
	}
}

func TestAMapSearchFallsBackToStreetGeocoding(t *testing.T) {
	h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v5/place/text" {
			io.WriteString(w, `{"status":"1","pois":[]}`)
			return
		}
		if r.URL.Path != "/v3/geocode/geo" || r.URL.Query().Get("address") != "兴南大道468号" {
			t.Errorf("incorrect geocoding request: %s", r.URL.Path)
		}
		io.WriteString(w, `{"status":"1","geocodes":[{"formatted_address":"广东省广州市番禺区兴南大道468号","province":"广东省","city":"广州市","district":"番禺区","location":"113.368476,23.013742"}]}`)
	})
	response := callLocation(h.search, "/?keyword=%E5%85%B4%E5%8D%97%E5%A4%A7%E9%81%93468%E5%8F%B7")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "广东省广州市番禺区兴南大道468号") {
		t.Fatalf("street fallback failed: %s", response.Body.String())
	}
}

func TestAMapReverseHandlesMunicipalityCityArray(t *testing.T) {
	h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("location") != "116.400000,39.900000" {
			t.Errorf("coordinates reversed: %s", r.URL.Query().Get("location"))
		}
		io.WriteString(w, `{"status":"1","regeocode":{"formatted_address":"北京市东城区测试地址","addressComponent":{"province":"北京市","city":[],"district":"东城区"}}}`)
	})
	response := callLocation(h.reverse, "/?longitude=116.4&latitude=39.9")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"city":"北京市"`) {
		t.Fatalf("municipality reverse failed: %s", response.Body.String())
	}
}

func TestAMapRejectsInvalidCoordinatesBeforeUpstream(t *testing.T) {
	for _, query := range []string{"longitude=NaN&latitude=23", "longitude=113&latitude=Inf", "longitude=181&latitude=23", "longitude=&latitude=23", "longitude=113&latitude=91"} {
		t.Run(query, func(t *testing.T) {
			h := testAMapHandler(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid coordinates reached provider") })
			if response := callLocation(h.reverse, "/?"+query); response.Code != 400 {
				t.Fatalf("invalid coordinate status=%d", response.Code)
			}
		})
	}
}

func TestAMapProviderErrorsDoNotExposeResponseOrCredentials(t *testing.T) {
	for _, body := range []string{`{"status":"0","info":"private-provider-detail test-web-key"}`, `not JSON test-private-code`} {
		h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
		response := callLocation(h.search, "/?keyword=test")
		if response.Code != 502 {
			t.Fatalf("provider error status=%d", response.Code)
		}
		if strings.Contains(response.Body.String(), "test-web-key") || strings.Contains(response.Body.String(), "test-private-code") || strings.Contains(response.Body.String(), "private-provider-detail") {
			t.Fatal("provider error leaked credentials or internal response")
		}
	}
}

func TestAMapWrongKeyTypeExplainsRequiredConfiguration(t *testing.T) {
	h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"0","infocode":"10009","info":"USERKEY_PLAT_NOMATCH"}`)
	})
	response := callLocation(h.search, "/?keyword=test")
	if response.Code != 502 || !strings.Contains(response.Body.String(), "Web 服务") || !strings.Contains(response.Body.String(), "10009") {
		t.Fatalf("wrong Key type needs an actionable explanation: %s", response.Body.String())
	}
}

func TestAMapEmptyResultsAreSuccessfulEmptyList(t *testing.T) {
	h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"1","pois":[],"geocodes":[]}`)
	})
	response := callLocation(h.search, "/?keyword=test")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"locations":[]`) {
		t.Fatalf("empty search should be a list: %s", response.Body.String())
	}
}

func TestAMapReadsEditedDotEnvWithoutRestart(t *testing.T) {
	path := t.TempDir() + "/.env"
	t.Setenv("AMAP_CONFIG_FILE", path)
	os.WriteFile(path, []byte("\ufeff# config\nAMAP_JS_KEY='first-key'\nAMAP_SECURITY_JS_CODE=first-code\n"), 0600)
	config, err := loadAMapConfig()
	if err != nil || config.JSKey != "first-key" {
		t.Fatalf("initial dotenv not read: %+v %v", config, err)
	}
	os.WriteFile(path, []byte("AMAP_JS_KEY=second-key # edited\nAMAP_WEB_KEY=web-key\nAMAP_SECURITY_JS_CODE=second-code\n"), 0600)
	config, err = loadAMapConfig()
	if err != nil || config.JSKey != "second-key" || config.WebKey != "web-key" {
		t.Fatalf("edited dotenv not reloaded: %+v %v", config, err)
	}
	t.Setenv("AMAP_JS_KEY", "override-key")
	config, _ = loadAMapConfig()
	if config.JSKey != "override-key" {
		t.Fatal("environment must override dotenv")
	}
}

func TestAMapSDKProxyReplacesClientCredentials(t *testing.T) {
	var receivedKey, receivedCode string
	h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.URL.Query().Get("key")
		receivedCode = r.URL.Query().Get("jscode")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	})
	router := mapSecurityRouter(h, 1)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/_AMapService/v4/maps?key=attacker&jscode=attacker", nil))
	if response.Code != 200 || receivedKey != "test-js-key" || receivedCode != "test-private-code" {
		t.Fatalf("SDK proxy credentials incorrect: %d %q %q", response.Code, receivedKey, receivedCode)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/_AMapService/https://example.com", nil))
	if response.Code != 404 {
		t.Fatal("proxy accepted arbitrary upstream path")
	}
}

func TestAMapRequestHonorsContextCancellation(t *testing.T) {
	h := testAMapHandler(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"status":"1"}`) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.amapJSON(ctx, "/v5/place/text", url.Values{}, amapConfig{WebKey: "test-key"}); err == nil {
		t.Fatal("cancelled requests must terminate")
	}
}

func TestLocationConfigurationWithoutKeys(t *testing.T) {
	t.Setenv("AMAP_CONFIG_FILE", t.TempDir()+"/absent.json")
	t.Setenv("AMAP_WEB_KEY", "")
	t.Setenv("AMAP_JS_KEY", "")
	t.Setenv("AMAP_SECURITY_JS_CODE", "")
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	authenticatedRouter(t, &Handlers{}).ServeHTTP(response, httptest.NewRequest("GET", "/api/maps/config", nil))
	if response.Code != 200 {
		t.Fatalf("map configuration status = %d; want 200", response.Code)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config["map_enabled"] != false || config["search_enabled"] != false {
		t.Fatalf("missing keys must disable unavailable capabilities: %v", config)
	}
}

func TestLocationSearchWithoutKeys(t *testing.T) {
	t.Setenv("AMAP_CONFIG_FILE", t.TempDir()+"/absent.json")
	t.Setenv("AMAP_WEB_KEY", "")
	response := httptest.NewRecorder()
	authenticatedRouter(t, &Handlers{}).ServeHTTP(response, httptest.NewRequest("POST", "/api/locations/search", strings.NewReader(`{"keyword":"实习单位"}`)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured search status = %d; want 503", response.Code)
	}
}

func TestLocationSearchRejectsEmptyKeyword(t *testing.T) {
	response := httptest.NewRecorder()
	authenticatedRouter(t, &Handlers{}).ServeHTTP(response, httptest.NewRequest("POST", "/api/locations/search", strings.NewReader(`{"keyword":" "}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("empty keyword status = %d; want 400", response.Code)
	}
}

func TestMapConfigurationDoesNotExposePrivateKeys(t *testing.T) {
	t.Setenv("AMAP_CONFIG_FILE", t.TempDir()+"/absent.json")
	t.Setenv("AMAP_WEB_KEY", "private-web-key")
	t.Setenv("AMAP_JS_KEY", "public-js-key")
	t.Setenv("AMAP_SECURITY_JS_CODE", "private-security-code")
	response := httptest.NewRecorder()
	authenticatedRouter(t, &Handlers{}).ServeHTTP(response, httptest.NewRequest("GET", "/api/maps/config", nil))
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, "public-js-key") {
		t.Fatalf("public SDK configuration unavailable: %s", body)
	}
	if strings.Contains(body, "private-web-key") || strings.Contains(body, "private-security-code") {
		t.Fatal("private credentials exposed to browser")
	}
}
