package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"xixunyunsign/platformauth"
)

// Existing business tests exercise their original handlers with a real system
// session. Production routes never bypass authentication when the store is nil.
type authenticatedTestRouter struct {
	*gin.Engine
	cookie string
}

func (r *authenticatedTestRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.Header.Set("Cookie", r.cookie)
	r.Engine.ServeHTTP(w, req)
}
func authenticatedRouter(t *testing.T, h *Handlers) *authenticatedTestRouter {
	t.Helper()
	s, m := newAuthFixture(t)
	if e := s.SendCode(context.Background(), "fixture@example.com", "register", "fixture"); e != nil {
		t.Fatal(e)
	}
	u, e := s.Register("fixture", "fixture@example.com", "Fixture-password-9", m.code)
	if e != nil {
		t.Fatal(e)
	}
	_, token, e := s.Login("fixture", "Fixture-password-9", "fixture")
	if e != nil {
		t.Fatal(e)
	}
	for _, account := range []string{"u", "v", "test"} {
		if e = s.Bind(u.ID, account, "1", "测试学校"); e != nil {
			t.Fatal(e)
		}
	}
	h.PlatformAuth = s
	return &authenticatedTestRouter{SetupRouter(h), authCookie + "=" + token}
}

type authTestMail struct{ code string }

func (m *authTestMail) Ready() bool { return true }
func (m *authTestMail) SendCode(_ context.Context, _, code, _ string) error {
	m.code = code
	return nil
}
func newAuthFixture(t *testing.T) (*platformauth.Store, *authTestMail) {
	t.Helper()
	t.Setenv("GEETEST_ENABLED", "false")
	db, e := sql.Open("sqlite3", filepath.Join(t.TempDir(), "auth.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	m := &authTestMail{}
	s, e := platformauth.New(db, m)
	if e != nil {
		t.Fatal(e)
	}
	return s, m
}

func TestAuthenticationStatusDisablesEmailRegistration(t *testing.T) {
	s, _ := newAuthFixture(t)
	r := SetupRouter(&Handlers{PlatformAuth: s})
	w := authRequest(r, "GET", "/api/auth/status", "", "")
	var status struct {
		Registration  bool `json:"registration"`
		EmailLogin    bool `json:"email_login"`
		OfficialLogin bool `json:"official_login"`
		SessionDays   int  `json:"session_days"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.Registration || status.EmailLogin || !status.OfficialLogin || status.SessionDays != 365 {
		t.Fatal("incorrect login capabilities", w.Code, w.Body)
	}
}
func authRequest(router http.Handler, method, path, body, cookie string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		r.Header.Set("Cookie", cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
func TestPlatformAuthenticationFlow(t *testing.T) {
	s, m := newAuthFixture(t)
	r := SetupRouter(&Handlers{PlatformAuth: s})
	for _, path := range []string{"/api/query?account=victim", "/api/auto-sign/tasks?account=victim", "/api/reports?account=victim", "/api/maps/config"} {
		if w := authRequest(r, "GET", path, "", ""); w.Code != 401 {
			t.Fatalf("anonymous %s = %d", path, w.Code)
		}
	}
	if e := s.SendCode(context.Background(), "a@example.com", "register", "fixture"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Register("alice", "a@example.com", "Correct-password-9", m.code); e != nil {
		t.Fatal(e)
	}
	w := authRequest(r, "POST", "/api/auth/login", `{"identity":"alice","password":"Correct-password-9","accept_terms":true}`, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("cookie protections missing")
	}
	cookie := cookies[0].Name + "=" + cookies[0].Value
	if w = authRequest(r, "GET", "/api/auth/self", "", cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = authRequest(r, "GET", "/api/query?account=victim", "", cookie); w.Code != 403 {
		t.Fatal("other account accessible", w.Code)
	}
	if w = authRequest(r, "POST", "/api/auth/logout", "{}", cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = authRequest(r, "GET", "/api/auth/self", "", cookie); w.Code != 401 {
		t.Fatal("session survived logout", w.Code)
	}
}
func TestAuthenticationRejectsCSRFAndMissingStore(t *testing.T) {
	s, _ := newAuthFixture(t)
	r := SetupRouter(&Handlers{PlatformAuth: s})
	for _, site := range []string{"https://evil.example", "http://example.com.evil.test"} {
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
		req.Header.Set("Origin", site)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("CSRF accepted", w.Code)
		}
	}
	if w := authRequest(SetupRouter(&Handlers{}), "GET", "/api/query?account=a", "", ""); w.Code != 503 {
		t.Fatal("missing store failed open", w.Code)
	}
}

type bindingAuthStub struct {
	calls int
	fail  bool
}

func (s *bindingAuthStub) Login(_, _, _ string) (string, error) {
	s.calls++
	if s.fail {
		return "", fmt.Errorf("连接失败")
	}
	return "private-upstream-token", nil
}
func TestBindingAndEveryAccountTransportRequireOwnership(t *testing.T) {
	s, m := newAuthFixture(t)
	create := func(name, email string) (platformauth.User, string) {
		t.Helper()
		if e := s.SendCode(context.Background(), email, "register", email); e != nil {
			t.Fatal(e)
		}
		u, e := s.Register(name, email, "Correct-password-9", m.code)
		if e != nil {
			t.Fatal(e)
		}
		_, token, e := s.Login(name, "Correct-password-9", name)
		if e != nil {
			t.Fatal(e)
		}
		return u, authCookie + "=" + token
	}
	alice, a := create("001241516129", "a@example.com")
	bob, b := create("bobby", "b@example.com")
	stub := &bindingAuthStub{fail: true}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	payload := `{"account":"001241516129","password":"upstream-password","school_id":"1","school_name":"学校"}`
	if w := authRequest(r, "POST", "/api/login", payload, a); w.Code != 401 || s.Owns(alice.ID, "001241516129") {
		t.Fatal("failed connection claimed account", w.Code)
	}
	stub.fail = false
	if w := authRequest(r, "POST", "/api/login", payload, a); w.Code != 200 || strings.Contains(w.Body.String(), "private-upstream-token") {
		t.Fatal(w.Code, w.Body)
	}
	if !s.Owns(alice.ID, "001241516129") {
		t.Fatal("not bound")
	}
	if w := authRequest(r, "POST", "/api/login", payload, b); w.Code != 403 || stub.calls != 2 || s.Owns(bob.ID, "001241516129") {
		t.Fatal("binding stolen or upstream overwritten", w.Code, stub.calls)
	}
	for _, test := range []struct{ method, path, body string }{
		{"GET", "/api/query?account=001241516129", ""},
		{"GET", "/api/reports?account=001241516129", ""},
		{"GET", "/api/auto-sign/tasks?account=001241516129", ""},
		{"POST", "/api/sign", `{"account":"001241516129"}`},
		{"POST", "/api/reports", `{"account":"001241516129"}`},
		{"POST", "/api/auto-sign/tasks/1/state", `{"account":"001241516129","enabled":false}`},
		{"DELETE", "/api/auto-sign/tasks/1", `{"account":"001241516129"}`},
	} {
		if w := authRequest(r, test.method, test.path, test.body, b); w.Code != 403 {
			t.Fatalf("%s %s bypassed: %d", test.method, test.path, w.Code)
		}
	}
	for _, path := range []string{"/api/sign/photos", "/api/reports/attachments"} {
		var buf bytes.Buffer
		form := multipart.NewWriter(&buf)
		form.WriteField("account", "001241516129")
		file, _ := form.CreateFormFile("file", "test.txt")
		file.Write([]byte("test"))
		form.Close()
		req := httptest.NewRequest("POST", path, &buf)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("Cookie", b)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("multipart bypass", path, w.Code)
		}
	}
}

func TestLegacyStudentNumberLinksSchoolAccount(t *testing.T) {
	s, m := newAuthFixture(t)
	stub := &bindingAuthStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	if e := s.SendCode(context.Background(), "student@example.com", "register", "student"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Register("001241516129", "student@example.com", "Correct-password-9", m.code); e != nil {
		t.Fatal(e)
	}
	_, token, e := s.Login("001241516129", "Correct-password-9", "student")
	if e != nil {
		t.Fatal(e)
	}
	cookie := authCookie + "=" + token
	w := authRequest(r, "GET", "/api/auth/self", "", cookie)
	var response struct {
		User struct {
			StudentNumber string `json:"student_number"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.User.StudentNumber != "001241516129" {
		t.Fatal("student number lost leading zeros", w.Body)
	}
	w = authRequest(r, "POST", "/api/login", `{"account":"241516129","password":"school-password","school_id":"1"}`, cookie)
	if w.Code != 400 || stub.calls != 0 {
		t.Fatal("mismatched student number reached upstream", w.Code, stub.calls)
	}
	w = authRequest(r, "POST", "/api/login", `{"account":"001241516129","password":"school-password","school_id":"1"}`, cookie)
	if w.Code != 200 || stub.calls != 1 || !s.Owns(responseUserID(t, s, token), "001241516129") {
		t.Fatal("matching student number not bound", w.Code, stub.calls)
	}
}
func responseUserID(t *testing.T, s *platformauth.Store, token string) int64 {
	t.Helper()
	u, e := s.Self(token)
	if e != nil {
		t.Fatal(e)
	}
	return u.ID
}
