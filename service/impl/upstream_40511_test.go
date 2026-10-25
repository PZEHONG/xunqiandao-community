package impl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
	"xixunyunsign/service"
)

func Test40511SessionRenewalAndSafeReplay(t *testing.T) {
	for _, mode := range []string{"read-numeric", "read-string", "sign-post", "rejected-renewal", "missing-credentials", "business-99999", "network-timeout"} {
		t.Run(mode, func(t *testing.T) {
			repo := &renewalRepo{token: "expired-40511-token", password: "private-password", school: "1"}
			if mode == "missing-credentials" {
				repo.password = ""
			}
			reads, posts, logins := 0, 0, 0
			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(previous)
			s := NewSignService(repo).(*SignServiceImpl)
			s.additionalUserData = func(string) (map[string]string, error) { return map[string]string{"school_id": "1"}, nil }
			s.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/login/api" {
					logins++
					return reportResponse(`{"code":20000,"data":{"token":"renewed-40511-token","school_id":1}}`), nil
				}
				if req.Method == "POST" {
					posts++
					req.ParseForm()
					if req.PostForm.Get("comment") != "original" {
						t.Error("request body lost")
					}
				} else {
					reads++
				}
				if mode == "business-99999" {
					return reportResponse(`{"code":99999,"message":"签到失败,请重试"}`), nil
				}
				if mode == "network-timeout" {
					return nil, fmt.Errorf("network timeout private-password")
				}
				if req.URL.Query().Get("token") == "expired-40511-token" || mode == "rejected-renewal" {
					if mode == "read-string" {
						return reportResponse(`{"code":"40511","message":"登录超时"}`), nil
					}
					return reportResponse(`{"code":40511,"message":"登录超时"}`), nil
				}
				return reportResponse(`{"code":20000,"data":{"mark_list":[{"key":0,"value":"上班"}]}}`), nil
			})}
			method, endpoint, body := "GET", "signin40/homepage", ""
			if mode == "sign-post" || mode == "business-99999" || mode == "network-timeout" {
				method, endpoint, body = "POST", "signin_rsa", "comment=original"
			}
			ctx := context.WithValue(context.Background(), service.SignTaskIDKey{}, int64(4))
			_, err := s.request(ctx, "40511-"+mode, method, endpoint, strings.NewReader(body), "application/x-www-form-urlencoded")
			switch mode {
			case "missing-credentials", "rejected-renewal":
				var failure *service.SignFailure
				if !errors.Is(err, service.ErrUpstreamLoginRequired) || !errors.As(err, &failure) || failure.Code != "40511" {
					t.Fatalf("lost auth failure code: %v", err)
				}
				want := 1
				if mode == "rejected-renewal" {
					want = 2
				}
				if reads != want || posts != 0 || logins != want-1 {
					t.Fatalf("unbounded renewal: reads=%d posts=%d logins=%d", reads, posts, logins)
				}
			case "business-99999", "network-timeout":
				if err == nil || posts != 1 || logins != 0 {
					t.Fatalf("unsafe replay: err=%v posts=%d logins=%d", err, posts, logins)
				}
			default:
				if err != nil || reads+posts != 2 || logins != 1 {
					t.Fatalf("40511 not renewed: %v reads=%d posts=%d logins=%d", err, reads, posts, logins)
				}
				if !strings.Contains(logs.String(), "[upstream_session]") || !strings.Contains(logs.String(), "code=40511") {
					t.Fatal("missing renewal diagnostics")
				}
			}
			for _, secret := range []string{"expired-40511-token", "renewed-40511-token", "private-password"} {
				if strings.Contains(logs.String(), secret) {
					t.Fatal("renewal logs leaked secret")
				}
			}
		})
	}
}
