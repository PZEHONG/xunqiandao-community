package impl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
	"xixunyunsign/service"
)

type renewalState struct {
	mu          sync.Mutex
	failedToken string
	retryAfter  time.Time
	err         error
}

var renewalStates sync.Map

func accountRenewalState(account string) *renewalState {
	value, _ := renewalStates.LoadOrStore(account, &renewalState{})
	return value.(*renewalState)
}

// Only explicit session rejections (40510/40511) permit a single renewal replay.
// Network timeouts and other business errors never permit replay.
func doUpstreamRequest(repo service.UserRepository, client *http.Client, req *http.Request, account string) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	code, err := upstreamSessionRejection(resp)
	if err != nil {
		return nil, err
	}
	if code == "" {
		return resp, nil
	}
	httpStatus := resp.StatusCode
	taskID, _ := req.Context().Value(service.SignTaskIDKey{}).(int64)
	log.Printf("[upstream_session] task_id=%d endpoint=%s code=%s stage=detected status=expired", taskID, req.URL.Path, code)
	sessionFailure := func(rejectedCode string, cause error) error {
		kind, message := "login_required", fmt.Sprintf("习讯云登录已失效（返回码：%s），请重新连接账号", rejectedCode)
		if !errors.Is(cause, service.ErrUpstreamLoginRequired) {
			kind, message = "renewal_failed", fmt.Sprintf("习讯云登录已失效（返回码：%s），自动续连失败，请稍后重新连接账号", rejectedCode)
		}
		return &service.SignFailure{Kind: kind, HTTPStatus: httpStatus, Code: rejectedCode, Message: message, Cause: cause}
	}
	resp.Body.Close()
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return nil, sessionFailure(code, service.ErrUpstreamLoginRequired)
	}
	credentials, ok := repo.(service.SessionCredentials)
	if !ok {
		return nil, sessionFailure(code, service.ErrUpstreamLoginRequired)
	}
	oldToken := req.URL.Query().Get("token")
	state := accountRenewalState(account)
	state.mu.Lock()
	token, _, _, readErr := repo.GetUser(account)
	if readErr != nil {
		state.mu.Unlock()
		return nil, sessionFailure(code, service.ErrUpstreamLoginRequired)
	}
	if token == "" || token == oldToken {
		if state.failedToken == oldToken && time.Now().Before(state.retryAfter) {
			err = state.err
		} else {
			password, school, credentialErr := credentials.SessionCredentials(account)
			if credentialErr != nil || password == "" || school == "" {
				err = service.ErrUpstreamLoginRequired
			} else {
				loginClient := *client
				loginClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
				auth := &AuthServiceImpl{userRepo: repo, httpClient: &loginClient}
				log.Printf("[upstream_session] task_id=%d endpoint=%s code=%s stage=renewal status=running", taskID, req.URL.Path, code)
				token, err = auth.login(req.Context(), account, password, school, false)
				if errors.Is(err, service.ErrUpstreamLoginRequired) {
					err = service.ErrUpstreamLoginRequired
				} else if err != nil {
					err = fmt.Errorf("习讯云暂时无法自动续连，请稍后重试")
				}
			}
			if err != nil && req.Context().Err() == nil {
				state.failedToken = oldToken
				state.retryAfter = time.Now().Add(30 * time.Second)
				state.err = err
			}
		}
	}
	state.mu.Unlock()
	if err != nil {
		log.Printf("[upstream_session] task_id=%d endpoint=%s code=%s stage=renewal status=failed error_class=%T", taskID, req.URL.Path, code, err)
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		return nil, sessionFailure(code, err)
	}
	log.Printf("[upstream_session] task_id=%d endpoint=%s code=%s stage=renewal status=success", taskID, req.URL.Path, code)
	if err = req.Context().Err(); err != nil {
		return nil, err
	}
	retry := req.Clone(req.Context())
	query := retry.URL.Query()
	query.Set("token", token)
	retry.URL.RawQuery = query.Encode()
	if retry.Header.Get("Authorization") == oldToken {
		retry.Header.Set("Authorization", token)
	}
	if req.GetBody != nil {
		retry.Body, err = req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("无法恢复请求内容，请重试")
		}
	}
	resp, err = client.Do(retry)
	if err != nil {
		return nil, err
	}
	retryCode, err := upstreamSessionRejection(resp)
	if err != nil {
		return nil, err
	}
	if retryCode != "" {
		resp.Body.Close()
		state.mu.Lock()
		current, _, _, readErr := repo.GetUser(account)
		if readErr == nil && current == token {
			state.failedToken = token
			state.retryAfter = time.Now().Add(30 * time.Second)
			state.err = service.ErrUpstreamLoginRequired
		}
		state.mu.Unlock()
		log.Printf("[upstream_session] task_id=%d endpoint=%s code=%s stage=replay status=rejected", taskID, req.URL.Path, retryCode)
		return nil, sessionFailure(retryCode, service.ErrUpstreamLoginRequired)
	}
	return resp, nil
}

func upstreamSessionRejection(resp *http.Response) (string, error) {
	payload, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	resp.Body.Close()
	if err != nil || len(payload) > 8<<20 {
		return "", fmt.Errorf("读取习讯云响应失败")
	}
	resp.Body = io.NopCloser(bytes.NewReader(payload))
	var envelope struct {
		Code json.Number `json:"code"`
	}
	if json.Unmarshal(payload, &envelope) == nil {
		switch code := envelope.Code.String(); code {
		case "40510", "40511":
			return code, nil
		}
	}
	return "", nil
}
