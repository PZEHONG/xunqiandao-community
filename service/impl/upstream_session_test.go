package impl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"xixunyunsign/service"
)

type renewalRepo struct {
	service.UserRepository
	mu       sync.Mutex
	token    string
	password string
	school   string
}

func (r *renewalRepo) GetUser(string) (string, string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.token, "", "", nil
}
func (r *renewalRepo) SaveUser(account, password, token, lat, lng, phone, number, name string, school float64, sex, class, entrance, graduation string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.token = token
	return nil
}
func (r *renewalRepo) SaveSessionCredentials(account, password, school string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.password = password
	r.school = school
	return nil
}
func (r *renewalRepo) SessionCredentials(string) (string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.password == "" {
		return "", "", fmt.Errorf("no credentials")
	}
	return r.password, r.school, nil
}

func TestExpiredUpstreamSessionRecoversConcurrentRequestsWithOneLogin(t *testing.T) {
	for _, code := range []string{"40510", "40511"} {
		t.Run(code, func(t *testing.T) {
			r := &renewalRepo{token: "expired", password: "secret", school: "1"}
			var logins, success atomic.Int32
			client := &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/login/api" {
					logins.Add(1)
					req.ParseForm()
					if req.PostForm.Get("password") != "secret" || req.PostForm.Get("school_id") != "1" {
						t.Error("lost renewal credentials")
					}
					return reportResponse(`{"code":20000,"data":{"token":"renewed","school_id":1}}`), nil
				}
				if req.URL.Query().Get("token") == "expired" {
					return reportResponse(fmt.Sprintf(`{"code":%s,"message":"登录超时"}`, code)), nil
				}
				if req.Header.Get("Authorization") != "renewed" {
					t.Error("stale authorization header")
				}
				success.Add(1)
				return reportResponse(`{"code":20000,"data":{"list":[],"count":0}}`), nil
			})}
			var group sync.WaitGroup
			for i := 0; i < 12; i++ {
				group.Add(1)
				go func() {
					defer group.Done()
					req, _ := http.NewRequest("GET", "https://api.xixunyun.com/Reports/StudentSearch?token=expired", nil)
					req.Header.Set("Authorization", "expired")
					resp, err := doUpstreamRequest(r, client, req, "concurrent-renewal-"+code)
					if err != nil {
						t.Error(err)
						return
					}
					resp.Body.Close()
				}()
			}
			group.Wait()
			if logins.Load() != 1 || success.Load() != 12 {
				t.Fatalf("logins=%d successful reads=%d", logins.Load(), success.Load())
			}
		})
	}
}

func TestUpstreamRecoveryReplaysOnlyExplicitAuthenticationRejections(t *testing.T) {
	for _, scenario := range []string{"expired-post", "network", "business-error", "rejected-new-token", "missing-credentials"} {
		t.Run(scenario, func(t *testing.T) {
			r := &renewalRepo{token: "expired", password: "secret", school: "1"}
			if scenario == "missing-credentials" {
				r.password = ""
			}
			calls, logins := 0, 0
			client := &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/login/api" {
					logins++
					return reportResponse(`{"code":20000,"data":{"token":"renewed","school_id":1}}`), nil
				}
				calls++
				body, _ := io.ReadAll(req.Body)
				if string(body) != "comment=original" {
					t.Error("request body changed on replay")
				}
				if scenario == "network" {
					return nil, fmt.Errorf("timeout")
				}
				if scenario == "business-error" {
					return reportResponse(`{"code":40001,"message":"business error"}`), nil
				}
				if req.URL.Query().Get("token") == "expired" || scenario == "rejected-new-token" {
					return reportResponse(`{"code":40510,"message":"会话已失效，请重新登录-1"}`), nil
				}
				return reportResponse(`{"code":20000,"data":"ok"}`), nil
			})}
			req, _ := http.NewRequestWithContext(context.Background(), "POST", "https://api.xixunyun.com/signin40/signin?token=expired", strings.NewReader("comment=original"))
			resp, err := doUpstreamRequest(r, client, req, "renewal-"+scenario)
			if resp != nil {
				resp.Body.Close()
			}
			if scenario == "expired-post" {
				if err != nil || calls != 2 || logins != 1 {
					t.Fatalf("recovery failed: %v calls=%d logins=%d", err, calls, logins)
				}
			}
			if scenario == "network" || scenario == "business-error" || scenario == "missing-credentials" {
				if calls != 1 || logins != 0 {
					t.Fatal("unsafe replay or unexpected login", calls, logins)
				}
			}
			if scenario == "rejected-new-token" {
				if err == nil || calls != 2 || logins != 1 {
					t.Fatal("unbounded retry or hidden auth failure", calls, logins, err)
				}
				repeat, _ := http.NewRequest("POST", "https://api.xixunyun.com/signin40/signin?token=renewed", strings.NewReader("comment=original"))
				if _, err = doUpstreamRequest(r, client, repeat, "renewal-"+scenario); err == nil || logins != 1 {
					t.Fatal("rejected renewed token caused repeated logins", logins, err)
				}
			}
		})
	}
}
