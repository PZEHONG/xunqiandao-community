// service/impl/auth_service.go
package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"xixunyunsign/service"
)

// AuthServiceImpl implements the AuthService interface.
type AuthServiceImpl struct {
	userRepo   service.UserRepository
	httpClient *http.Client // Keep http client internal for now
}

// NewAuthService creates a new AuthService implementation.
func NewAuthService(repo service.UserRepository) service.AuthService {
	// Return the exported type
	return &AuthServiceImpl{
		userRepo:   repo,
		httpClient: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

// Login performs the user login operation.
func (a *AuthServiceImpl) Login(account, password, schoolID string) (string, error) {
	return a.LoginContext(context.Background(), account, password, schoolID)
}

func (a *AuthServiceImpl) LoginContext(ctx context.Context, account, password, schoolID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	state := accountRenewalState(account)
	for !state.mu.TryLock() {
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	defer state.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return "", e
	}
	token, err := a.login(ctx, account, password, schoolID, true)
	if err == nil {
		state.failedToken = ""
		state.retryAfter = time.Time{}
		state.err = nil
	}
	return token, err
}

func (a *AuthServiceImpl) login(ctx context.Context, account, password, schoolID string, remember bool) (string, error) {
	apiURL := "https://api.xixunyun.com/login/api"
	data := url.Values{}
	data.Set("app_version", "5.1.3")
	data.Set("registration_id", "")
	data.Set("uuid", "fd9dc13a49cc850c")
	data.Set("request_source", "3")
	data.Set("platform", "2")
	data.Set("mac", "7C:F3:1B:BB:F1:C4")
	data.Set("password", password)
	data.Set("system", "10")
	data.Set("school_id", schoolID)
	data.Set("model", "LM-G820")
	data.Set("app_id", "cn.vanber.xixunyun.saas")
	data.Set("account", account)
	data.Set("key", "")

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	q := req.URL.Query()
	q.Add("from", "app")
	q.Add("version", "5.1.3")
	q.Add("platform", "android")
	q.Add("entrance_year", "0")
	q.Add("graduate_year", "0")
	q.Add("school_id", schoolID)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "okhttp/3.8.0")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("习讯云服务暂时不可用")
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return "", errors.New("读取习讯云响应失败")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if code, ok := result["code"].(float64); !ok || code != 20000 {
		message, _ := result["message"].(string)
		return "", fmt.Errorf("%w：%s", service.ErrUpstreamLoginRequired, message)
	}

	dataMap, ok := result["data"].(map[string]interface{})
	if !ok {
		return "", errors.New("响应数据格式错误")
	}
	token, ok := dataMap["token"].(string)
	if !ok || strings.TrimSpace(token) == "" {
		return "", errors.New("获取 token 失败")
	}
	returnedSchool, validSchool := dataMap["school_id"].(float64)
	expectedSchool, schoolErr := strconv.ParseFloat(schoolID, 64)
	if !validSchool || schoolErr != nil || returnedSchool != expectedSchool || returnedSchool <= 0 {
		return "", errors.New("习讯云返回的学校信息与所选学校不一致")
	}

	err = service.CommitLogin(ctx, func() error {
		if remember {
			if credentials, ok := a.userRepo.(service.SessionCredentials); ok {
				if err := credentials.SaveSessionCredentials(account, password, schoolID); err != nil {
					return errors.New("无法加密保存自动续连凭证，请检查本机存储")
				}
			}
		}
		// The legacy password column stays empty; recoverable credentials are encrypted.
		err := a.userRepo.SaveUser(
			account, "", token,
			"", "", // 经纬度为空
			getString(dataMap, "bind_phone"),
			getString(dataMap, "user_number"),
			getString(dataMap, "user_name"),
			returnedSchool,
			getString(dataMap, "sex"),
			getString(dataMap, "class_name"),
			getString(dataMap, "entrance_year"),
			getString(dataMap, "graduation_year"),
		)
		if err != nil {
			return fmt.Errorf("保存用户信息失败: %v", err)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return token, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
