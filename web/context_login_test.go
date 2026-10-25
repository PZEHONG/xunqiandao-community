package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReconnectSharesOfficialLoginRateLimit(t *testing.T) {
	s, _ := newAuthFixture(t)
	_, token, e := s.LoginOfficial("limited001", "1", "", "", "fixture", func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	stub := &bindingAuthStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	body := `{"account":"limited001","password":"password","school_id":"1","accept_terms":true}`
	for i := 1; i < 16; i++ {
		path, cookie := "/api/login", authCookie+"="+token
		if i%2 == 0 {
			path, cookie = "/api/auth/official", ""
		}
		w := authRequest(r, "POST", path, body, cookie)
		want := 200
		if i == 15 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d via %s: status %d expected %d", i, path, w.Code, want)
		}
	}
	if stub.calls != 14 {
		t.Fatal("throttle reached upstream", stub.calls)
	}
}

type contextLoginStub struct {
	ctx   context.Context
	calls int
}

func (s *contextLoginStub) Login(_, _, _ string) (string, error) { s.calls++; return "token", nil }
func (s *contextLoginStub) LoginContext(ctx context.Context, _, _, _ string) (string, error) {
	s.ctx = ctx
	s.calls++
	return "token", nil
}

func TestReconnectPropagatesRequestContext(t *testing.T) {
	s, _ := newAuthFixture(t)
	_, token, e := s.LoginOfficial("context001", "1", "", "", "fixture", func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	stub := &contextLoginStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request marker")
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"account":"context001","password":"password","school_id":"1"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authCookie+"="+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 || stub.calls != 1 || stub.ctx == nil || stub.ctx.Value(contextKey{}) != "request marker" {
		t.Fatal("request context not propagated", w.Code, stub.calls)
	}
}
