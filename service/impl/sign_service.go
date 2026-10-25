// service/impl/sign_service.go
package impl

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"xixunyunsign/service"
	"xixunyunsign/utils"
)

// SignServiceImpl implements the SignService interface.
type SignServiceImpl struct {
	userRepo           service.UserRepository
	client             *http.Client
	additionalUserData func(string) (map[string]string, error)
}

// NewSignService creates a new SignService implementation.
func NewSignService(repo service.UserRepository) service.SignService {
	// Return the exported type
	return &SignServiceImpl{
		userRepo:           repo,
		client:             &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		additionalUserData: utils.GetAdditionalUserData,
	}
}

// SignIn performs the sign-in operation for a user.
func (s *SignServiceImpl) SignIn(account, address, addressName, latitude, longitude string) (string, error) {
	return s.SignInWithOptions(context.Background(), account, address, addressName, latitude, longitude, service.SignOptions{Remark: "0"})
}

func signInput(message string) error { return fmt.Errorf("%w：%s", service.ErrSignInput, message) }

// All requests use the connected account's school context. Transport errors
// are deliberately replaced because net/http errors include the token URL.
func (s *SignServiceImpl) request(ctx context.Context, account, method, path string, body io.Reader, contentType string) (json.RawMessage, error) {
	return s.requestWithSchool(ctx, account, "", method, path, body, contentType)
}
func (s *SignServiceImpl) requestWithSchool(ctx context.Context, account, expectedSchool, method, path string, body io.Reader, contentType string) (json.RawMessage, error) {
	if strings.TrimSpace(account) == "" || s.userRepo == nil {
		return nil, service.ErrSignAccount
	}
	token, _, _, err := s.userRepo.GetUser(account)
	if err != nil || token == "" {
		return nil, service.ErrSignAccount
	}
	meta, err := s.additionalUserData(account)
	if err != nil || meta["school_id"] == "" {
		return nil, fmt.Errorf("学校信息不完整，请重新连接账号")
	}
	if expectedSchool != "" {
		expected, e1 := strconv.ParseFloat(expectedSchool, 64)
		actual, e2 := strconv.ParseFloat(meta["school_id"], 64)
		if e1 != nil || e2 != nil || expected != actual || math.IsNaN(expected) || math.IsInf(expected, 0) {
			return nil, signInput("学校信息已变化，请重新确认每日任务")
		}
	}
	q := url.Values{"token": {token}, "from": {"app"}, "version": {"5.1.3"}, "platform": {"android"}, "entrance_year": {meta["entrance_year"]}, "graduate_year": {meta["graduation_year"]}, "school_id": {meta["school_id"]}}
	if path == "signin40/homepage" {
		q.Set("month_date", time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01"))
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.xixunyun.com/"+path+"?"+q.Encode(), body)
	if err != nil {
		return nil, fmt.Errorf("无法创建签到请求")
	}
	req.Header.Set("User-Agent", "okhttp/3.8.0")
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	taskID, _ := ctx.Value(service.SignTaskIDKey{}).(int64)
	started := time.Now()
	diagnosticHTTP, diagnosticCode, diagnosticMessage, diagnosticOutcome := 0, "", "", "success"
	defer func() {
		log.Printf("[sign_upstream] task_id=%d endpoint=%s method=%s http_status=%d code=%s outcome=%s duration_ms=%d message=%q", taskID, path, method, diagnosticHTTP, diagnosticCode, diagnosticOutcome, time.Since(started).Milliseconds(), diagnosticMessage)
	}()
	resp, err := doUpstreamRequest(s.userRepo, s.client, req, account)
	if err != nil {
		diagnosticOutcome = "transport_error"
		var sessionFailure *service.SignFailure
		if errors.As(err, &sessionFailure) {
			diagnosticOutcome, diagnosticHTTP, diagnosticCode, diagnosticMessage = sessionFailure.Kind, sessionFailure.HTTPStatus, sessionFailure.Code, sessionFailure.Message
			return nil, err
		}
		if errors.Is(err, service.ErrUpstreamLoginRequired) {
			diagnosticOutcome = "login_required"
			return nil, &service.SignFailure{Kind: "login_required", Code: "40510", Message: "习讯云会话已失效（40510），请重新连接账号", Cause: err}
		}
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				diagnosticOutcome = "deadline_exceeded"
			} else {
				diagnosticOutcome = "cancelled"
			}
			return nil, ctx.Err()
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			diagnosticOutcome = "timeout"
			return nil, &service.SignFailure{Kind: "timeout", Message: "习讯云请求超时，未确认成功；可能已提交，请先查看官方签到记录，系统不会自动重试"}
		}
		return nil, &service.SignFailure{Kind: "transport_error", Message: "连接习讯云失败，未确认成功；可能已提交，请先查看官方签到记录，系统不会自动重试"}
	}
	diagnosticHTTP = resp.StatusCode
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		diagnosticOutcome = "response_read_error"
		return nil, &service.SignFailure{Kind: "response_read_error", HTTPStatus: resp.StatusCode, Message: "读取签到结果失败，未确认成功；可能已提交，请先查看官方签到记录"}
	}
	var result struct {
		Code    json.Number     `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(payload, &result) != nil {
		diagnosticOutcome = "invalid_json"
		return nil, &service.SignFailure{Kind: "invalid_json", HTTPStatus: resp.StatusCode, Message: "习讯云响应格式异常，未确认成功；可能已提交，请先查看官方签到记录"}
	}
	diagnosticCode = result.Code.String()
	diagnosticMessage = safeSignDiagnosticMessage(result.Message, req, account, token)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || result.Code.String() != "20000" {
		diagnosticOutcome = "upstream_rejected"
		message := diagnosticMessage
		if message == "" {
			message = "习讯云未接受本次操作"
		}
		if diagnosticCode != "" {
			message = fmt.Sprintf("%s（返回码：%s）", message, diagnosticCode)
		}
		return nil, &service.SignFailure{Kind: "upstream_rejected", HTTPStatus: resp.StatusCode, Code: diagnosticCode, Message: message}
	}
	return result.Data, nil
}

func (s *SignServiceImpl) Remarks(ctx context.Context, account string) ([]service.SignRemark, error) {
	raw, err := s.request(ctx, account, http.MethodGet, "signin40/homepage", nil, "")
	if err != nil {
		return nil, err
	}
	var data struct {
		Marks []struct {
			Key   json.RawMessage `json:"key"`
			Value string          `json:"value"`
		} `json:"mark_list"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return nil, fmt.Errorf("无法读取签到备注，请重试")
	}
	marks := make([]service.SignRemark, 0, len(data.Marks))
	seen := map[string]bool{}
	for _, mark := range data.Marks {
		var key string
		if json.Unmarshal(mark.Key, &key) != nil {
			var number json.Number
			if json.Unmarshal(mark.Key, &number) != nil {
				continue
			}
			key = number.String()
		}
		if key == "" || mark.Value == "" || seen[key] {
			continue
		}
		seen[key] = true
		marks = append(marks, service.SignRemark{Key: key, Value: mark.Value})
	}
	if len(marks) == 0 {
		return nil, fmt.Errorf("账号未返回可用签到备注，请在习讯云检查实习安排")
	}
	return marks, nil
}

func validSignPhotoURI(value string) bool {
	u, err := url.Parse(value)
	return err == nil && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && u.Scheme == "" && u.Host == "" && u.RawQuery == "" && u.Fragment == "" && !strings.ContainsAny(value, ",\r\n\x00\\")
}

func (s *SignServiceImpl) SignInWithOptions(ctx context.Context, account, address, addressName, latitude, longitude string, options service.SignOptions) (string, error) {
	lat, latErr := strconv.ParseFloat(latitude, 64)
	lon, lonErr := strconv.ParseFloat(longitude, 64)
	if latErr != nil || lonErr != nil || math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return "", signInput("请明确提供已确认地点的完整有效坐标；不使用平台或数据库位置")
	}
	if strings.TrimSpace(address) == "" {
		return "", signInput("请确认完整签到地址")
	}
	if utf8.RuneCountInString(options.Comment) > 1000 {
		return "", signInput("签到说明最多 1000 字")
	}
	if len(options.Photos) > service.MaxSignPhotos {
		return "", signInput("最多添加 3 张照片")
	}
	for _, photo := range options.Photos {
		if !validSignPhotoURI(photo) {
			return "", signInput("照片地址无效，请重新选择照片")
		}
	}
	if options.Remark == "" {
		options.Remark = "0"
	}

	encryptedLat, err := rsaEncrypt([]byte(latitude))
	if err != nil {
		return "", fmt.Errorf("加密纬度失败: %v", err)
	}
	encryptedLong, err := rsaEncrypt([]byte(longitude))
	if err != nil {
		return "", fmt.Errorf("加密经度失败: %v", err)
	}

	// 从 address 提取省份和城市
	province, city, err := extractProvinceAndCity(address)
	if err != nil {
		return "", signInput("地址缺少省市，请重新搜索并确认完整地址")
	}
	marks, err := s.Remarks(ctx, account)
	if err != nil {
		return "", err
	}
	available := false
	for _, mark := range marks {
		if mark.Key == options.Remark && (options.ExpectedRemarkLabel == "" || options.ExpectedRemarkLabel == mark.Value) {
			available = true
			break
		}
	}
	if !available {
		return "", signInput("所选签到备注已不可用，请刷新备注后重新选择")
	}

	data := url.Values{}
	data.Set("address", address)
	data.Set("province", province)
	data.Set("city", city)
	data.Set("latitude", encryptedLat)
	data.Set("longitude", encryptedLong)
	data.Set("remark", options.Remark)
	data.Set("comment", options.Comment)
	if len(options.Photos) > 0 {
		data.Set("file", strings.Join(options.Photos, ","))
	}
	data.Set("address_name", addressName)
	data.Set("change_sign_resource", "0")
	if options.BeforeSubmit != nil {
		if err := options.BeforeSubmit(); err != nil {
			return "", err
		}
	}

	if _, err = s.requestWithSchool(ctx, account, options.ExpectedSchoolID, http.MethodPost, "signin_rsa", strings.NewReader(data.Encode()), "application/x-www-form-urlencoded"); err != nil {
		return "", err
	}
	return "签到成功", nil
}

func (s *SignServiceImpl) UploadPhoto(ctx context.Context, account, filename string, file io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(file, service.MaxSignPhotoSize+1))
	if err != nil {
		return "", fmt.Errorf("读取照片失败")
	}
	if len(data) == 0 || int64(len(data)) > service.MaxSignPhotoSize {
		return "", signInput("单张照片需为 1 字节至 10 MB")
	}
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return "", signInput("请选择 JPG、PNG、GIF 或 WebP 图片")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("addFile[]", filepath.Base(filename))
	if err != nil {
		return "", signInput("照片名称无效")
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	if err = writer.WriteField("input_name", "addFile"); err != nil {
		return "", err
	}
	if err = writer.WriteField("business", "signin"); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	raw, err := s.request(ctx, account, http.MethodPost, "file/form", &body, writer.FormDataContentType())
	if err != nil {
		return "", err
	}
	var result struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return "", fmt.Errorf("照片上传未返回有效地址，请重试")
	}
	// file/form returns either a relative path or an absolute OSS storage URL.
	// The official record page prefixes file_domain, so signin needs the path.
	if u, parseErr := url.Parse(result.URI); parseErr == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		switch u.Host {
		case "vanber-production.oss-cn-shanghai-internal.aliyuncs.com", "vanber-production.oss-cn-shanghai.aliyuncs.com":
			result.URI = u.EscapedPath()
		}
	}
	if !validSignPhotoURI(result.URI) {
		return "", fmt.Errorf("照片上传未返回有效地址，请重试")
	}
	return result.URI, nil
}

// rsaEncrypt 使用公钥加密数据
func rsaEncrypt(origData []byte) (string, error) {
	publicKey := `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDlYsiV3DsG+t8OFMLyhdmG2P2J
4GJwmwb1rKKcDZmTxEphPiYTeFIg4IFEiqDCATAPHs8UHypphZTK6LlzANyTzl9L
jQS6BYVQk81LhQ29dxyrXgwkRw9RdWaMPtcXRD4h6ovx6FQjwQlBM5vaHaJOHhEo
rHOSyd/deTvcS+hRSQIDAQAB
-----END PUBLIC KEY-----`
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return "", errors.New("公钥解码失败")
	}
	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pub, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("解析公钥类型失败")
	}
	encryptedData, err := rsa.EncryptPKCS1v15(rand.Reader, pub, origData)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encryptedData), nil
}

func extractProvinceAndCity(address string) (string, string, error) {
	for _, city := range []string{"北京市", "天津市", "上海市", "重庆市"} {
		if strings.HasPrefix(address, city) {
			return city, city, nil
		}
	}
	re := regexp.MustCompile(`^(.+?(?:省|自治区|特别行政区))(.+?(?:市|自治州|地区|盟))`)
	matches := re.FindStringSubmatch(address)
	if len(matches) == 3 {
		return matches[1], matches[2], nil
	}
	return "", "", fmt.Errorf("无法提取省份和城市")
}

// Logs use response metadata only; request URLs, tokens and form values are excluded.
func safeSignDiagnosticMessage(message string, req *http.Request, account, token string) string {
	secrets := []string{account, token}
	if req.GetBody != nil && strings.HasPrefix(req.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if body, err := req.GetBody(); err == nil {
			payload, readErr := io.ReadAll(io.LimitReader(body, 2<<20))
			body.Close()
			if readErr == nil {
				if values, err := url.ParseQuery(string(payload)); err == nil {
					for _, key := range []string{"address", "address_name", "province", "city", "latitude", "longitude", "comment", "file"} {
						secrets = append(secrets, values.Get(key))
					}
				}
			}
		}
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[已隐藏]")
			message = strings.ReplaceAll(message, url.QueryEscape(secret), "[已隐藏]")
		}
	}
	message = diagnosticURL.ReplaceAllString(message, "[URL已隐藏]")
	message = diagnosticIdentifier.ReplaceAllString(message, "[标识已隐藏]")
	runes := []rune(message)
	if len(runes) > 240 {
		message = string(runes[:240]) + "…"
	}
	return message
}

var diagnosticURL = regexp.MustCompile("https?://[^\\s\"<>]+")
var diagnosticIdentifier = regexp.MustCompile("[A-Za-z0-9_./:=?%&+@-]{6,}")
