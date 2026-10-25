package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"xixunyunsign/platformauth"
	"xixunyunsign/service"
)

func TestOfficialLoginVerifiesBeforeCreatingSession(t *testing.T) {
	s, _ := newAuthFixture(t)
	stub := &bindingAuthStub{fail: true}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	body := `{"account":"001241516129","password":"school-password","school_id":"1","school_name":"测试学校","accept_terms":true}`
	w := authRequest(r, "POST", "/api/auth/official", body, "")
	if w.Code != 401 || len(w.Result().Cookies()) != 0 || stub.calls != 1 {
		t.Fatalf("failed official credentials: %d, calls=%d", w.Code, stub.calls)
	}
	stub.fail = false
	w = authRequest(r, "POST", "/api/auth/official", body, "")
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal(w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "private-upstream-token") || strings.Contains(w.Body.String(), "school-password") {
		t.Fatal("credentials disclosed")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.MaxAge <= 0 {
		t.Fatal("invalid session cookie")
	}
	u, e := s.Self(cookie.Value)
	if e != nil || u.StudentNumber != "001241516129" || u.Email != "" || !s.Owns(u.ID, u.StudentNumber) {
		t.Fatal("not logged in and bound", u, e)
	}
	w = authRequest(r, "GET", "/api/auth/self", "", cookie.String())
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"school_name":"测试学校"`) {
		t.Fatal("binding not restored", w.Body)
	}
	firstID := u.ID
	w = authRequest(r, "POST", "/api/auth/official", body, cookie.String())
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if _, e = s.Self(cookie.Value); e == nil {
		t.Fatal("previous browser session not revoked")
	}
	u, e = s.Self(w.Result().Cookies()[0].Value)
	if e != nil || u.ID != firstID {
		t.Fatal("duplicate direct user", u, e)
	}
	before := stub.calls
	w = authRequest(r, "POST", "/api/auth/official", strings.Replace(body, `"school_id":"1"`, `"school_id":"2"`, 1), "")
	if w.Code != 409 || stub.calls != before {
		t.Fatal("different school overwrote account", w.Code, stub.calls)
	}
}

type officialSchoolQuery struct {
	service.QueryService
	calls int
}

func (q *officialSchoolQuery) SearchSchool(string) ([]service.SchoolInfo, error) {
	q.calls++
	items := []service.SchoolInfo{}
	for i := 0; i < 60; i++ {
		items = append(items, service.SchoolInfo{SchoolID: fmt.Sprint(i + 1), SchoolName: fmt.Sprintf("学校 %d", i+1)})
	}
	return items, nil
}
func TestOfficialSchoolSearchIsPublicLimitedAndDoesNotExposeBusinessData(t *testing.T) {
	s, _ := newAuthFixture(t)
	query := &officialSchoolQuery{}
	r := SetupRouter(&Handlers{PlatformAuth: s, QueryService: query})
	if w := authRequest(r, "GET", "/api/auth/schools?school_name=x", "", ""); w.Code != 400 || query.calls != 0 {
		t.Fatal("invalid search reached provider")
	}
	for i := 0; i < 31; i++ {
		w := authRequest(r, "GET", "/api/auth/schools?school_name=Test", "", "")
		want := 200
		if i == 30 {
			want = 429
		}
		if w.Code != want {
			t.Fatal("school search limit", i, w.Code)
		}
		if i == 0 {
			var data struct {
				Schools []service.SchoolInfo `json:"schools"`
			}
			if json.Unmarshal(w.Body.Bytes(), &data) != nil || len(data.Schools) != 50 {
				t.Fatal("school results unbounded")
			}
		}
	}
	if query.calls != 30 {
		t.Fatal("search throttle reached provider", query.calls)
	}
	if w := authRequest(r, "GET", "/api/search_school?school_name=Test", "", ""); w.Code != 401 {
		t.Fatal("business route made public")
	}
}
func TestConnectedLoginCannotBypassOfficialSchoolGuard(t *testing.T) {
	db, e := sql.Open("sqlite3", filepath.Join(t.TempDir(), "legacy.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec("CREATE TABLE users(account TEXT PRIMARY KEY,school_id REAL,token TEXT); INSERT INTO users VALUES('001123',1.0,'legacy-token')"); e != nil {
		t.Fatal(e)
	}
	m := &authTestMail{}
	s, e := platformauth.New(db, m)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SendCode(context.Background(), "owner@example.com", "register", "ip"); e != nil {
		t.Fatal(e)
	}
	u, e := s.Register("001123", "owner@example.com", "Correct-password-9", m.code)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Bind(u.ID, "001123", "1", "学校"); e != nil {
		t.Fatal(e)
	}
	_, token, e := s.Login("001123", "Correct-password-9", "ip")
	if e != nil {
		t.Fatal(e)
	}
	stub := &bindingAuthStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	w := authRequest(r, "POST", "/api/login", `{"account":"001123","password":"school-password","school_id":"2"}`, authCookie+"="+token)
	if w.Code != 409 || stub.calls != 0 {
		t.Fatal("connected login bypassed school guard", w.Code, stub.calls)
	}
	w = authRequest(r, "POST", "/api/login", `{"account":"001123","password":"school-password","school_id":"1"}`, authCookie+"="+token)
	if w.Code != 200 || stub.calls != 1 {
		t.Fatal("matching legacy school not usable", w.Code, stub.calls)
	}
}

func TestOfficialLoginReusesOnlyVerifiedExistingBinding(t *testing.T) {
	s, m := newAuthFixture(t)
	if e := s.SendCode(context.Background(), "student@example.com", "register", "ip"); e != nil {
		t.Fatal(e)
	}
	u, e := s.Register("001241516129", "student@example.com", "Correct-password-9", m.code)
	if e != nil {
		t.Fatal(e)
	}
	stub := &bindingAuthStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	body := `{"account":"001241516129","password":"school-password","school_id":"1","school_name":"测试学校","accept_terms":true}`
	w := authRequest(r, "POST", "/api/auth/official", body, "")
	if w.Code != 409 || stub.calls != 0 {
		t.Fatal("unbound email identity claimed", w.Code, stub.calls)
	}
	if e = s.Bind(u.ID, u.StudentNumber, "1", "测试学校"); e != nil {
		t.Fatal(e)
	}
	w = authRequest(r, "POST", "/api/auth/official", body, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	actual, e := s.Self(w.Result().Cookies()[0].Value)
	if e != nil || actual.ID != u.ID || actual.Email != u.Email {
		t.Fatal("existing account not reused", actual, e)
	}
	stub.fail = true
	w = authRequest(r, "POST", "/api/auth/official", body, "")
	if w.Code != 401 || len(w.Result().Cookies()) != 0 {
		t.Fatal("bound identity skips verification", w.Code)
	}
}

func TestOfficialLoginRateLimitAndCrossOrigin(t *testing.T) {
	s, _ := newAuthFixture(t)
	stub := &bindingAuthStub{fail: true}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	body := `{"account":"001241516129","password":"bad","school_id":"1","accept_terms":true}`
	for i := 0; i < 16; i++ {
		w := authRequest(r, "POST", "/api/auth/official", body, "")
		want := 401
		if i == 15 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d status %d", i, w.Code)
		}
	}
	if stub.calls != 15 {
		t.Fatal("throttle occurs after upstream", stub.calls)
	}
	req := httptest.NewRequest("POST", "/api/auth/official", strings.NewReader(body))
	req.Header.Set("Origin", "https://other.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-origin login accepted", w.Code)
	}
}
