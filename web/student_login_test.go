package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
	"xixunyunsign/service"
)

type studentLoginStub struct {
	account, password, school string
	calls                     int
	fail                      bool
}

type delayedStudentQuery struct {
	service.QueryService
	entered chan struct{}
	release chan struct{}
}

func (q *delayedStudentQuery) QuerySignInfo(string) (map[string]interface{}, error) {
	close(q.entered)
	<-q.release
	return map[string]interface{}{}, nil
}

func TestLateBusinessResponseCannotReplaceNewLoginCookie(t *testing.T) {
	s, _ := newAuthFixture(t)
	query := &delayedStudentQuery{entered: make(chan struct{}), release: make(chan struct{})}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: &studentLoginStub{}, QueryService: query})
	w := authRequest(r, "POST", "/api/auth/official", `{"account":"001123","password":"123456","school_id":"1","accept_terms":true}`, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	old := w.Result().Cookies()[0]
	done := make(chan []*http.Cookie, 1)
	go func() {
		response := authRequest(r, "GET", "/api/query?account=001123", "", old.String())
		done <- response.Result().Cookies()
	}()
	select {
	case <-query.entered:
	case <-time.After(2 * time.Second):
		close(query.release)
		t.Fatal("business request did not start")
	}
	w = authRequest(r, "POST", "/api/auth/login", `{"identity":"001123","password":"123456","accept_terms":true}`, old.String())
	close(query.release)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if cookies := <-done; len(cookies) != 0 {
		t.Fatal("late response can overwrite the new session with a revoked cookie")
	}
	if _, err := s.Self(w.Result().Cookies()[0].Value); err != nil {
		t.Fatal("new session invalid", err)
	}
}

func (s *studentLoginStub) Login(account, password, school string) (string, error) {
	s.account, s.password, s.school = account, password, school
	s.calls++
	if s.fail {
		return "", errors.New("private upstream error")
	}
	return "private-upstream-token", nil
}

func TestStudentLoginReusesSavedSchoolAndRefreshesUpstream(t *testing.T) {
	s, _ := newAuthFixture(t)
	stub := &studentLoginStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	w := authRequest(r, "POST", "/api/auth/official", `{"account":"001123","password":"123456","school_id":"1","school_name":"测试学校","accept_terms":true}`, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	old := w.Result().Cookies()[0]
	u, err := s.Self(old.Value)
	if err != nil {
		t.Fatal(err)
	}
	w = authRequest(r, "POST", "/api/auth/login", `{"identity":"001123","password":"123456","accept_terms":true}`, old.String())
	if w.Code != 200 {
		t.Fatal("student-only login failed", w.Code, w.Body)
	}
	if stub.calls != 2 || stub.account != "001123" || stub.password != "123456" || stub.school != "1" {
		t.Fatal("login did not refresh upstream with the saved school", stub)
	}
	cookie := w.Result().Cookies()[0]
	if cookie.MaxAge < 365*24*60*60 || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("not a persistent protected cookie", cookie)
	}
	current, err := s.Self(cookie.Value)
	if err != nil || current.ID != u.ID || current.Email != "" {
		t.Fatal("identity not reused", current, err)
	}
	if _, err = s.Self(old.Value); err == nil {
		t.Fatal("old browser session survived replacement")
	}
	w = authRequest(r, "GET", "/api/auth/self", "", cookie.String())
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"school_name":"测试学校"`) || len(w.Result().Cookies()) != 1 {
		t.Fatal("restoration did not return binding and renewed cookie", w.Code, w.Body)
	}
	stub.fail = true
	w = authRequest(r, "POST", "/api/auth/login", `{"identity":"001123","password":"wrong","accept_terms":true}`, cookie.String())
	if w.Code != 401 || len(w.Result().Cookies()) != 0 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("failed login created session or leaked error", w.Code, w.Body)
	}
	if _, err = s.Self(cookie.Value); err != nil {
		t.Fatal("failed login revoked current session", err)
	}
}

func TestRegistrationAndEmailAuthenticationAreUnavailable(t *testing.T) {
	s, m := newAuthFixture(t)
	r := SetupRouter(&Handlers{PlatformAuth: s})
	for _, path := range []string{"/api/auth/register", "/api/auth/code", "/api/auth/reset"} {
		w := authRequest(r, "POST", path, `{"email":"student@example.com","purpose":"register"}`, "")
		if w.Code != 410 || m.code != "" {
			t.Fatal("email registration flow still available", path, w.Code)
		}
	}
	if err := s.SendCode(context.Background(), "student@example.com", "register", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register("001123", "student@example.com", "Correct-password-9", m.code); err != nil {
		t.Fatal(err)
	}
	w := authRequest(r, "POST", "/api/auth/login", `{"identity":"student@example.com","password":"Correct-password-9","accept_terms":true}`, "")
	if w.Code != 401 || len(w.Result().Cookies()) != 0 {
		t.Fatal("email login still accepted", w.Code)
	}
}
