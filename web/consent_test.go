package web

import (
	"strings"
	"testing"
)

func TestFirstLoginRequiresExplicitConsentBeforeUpstream(t *testing.T) {
	s, _ := newAuthFixture(t)
	stub := &studentLoginStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	body := `{"account":"001123","password":"school-password","school_id":"1"}`
	w := authRequest(r, "POST", "/api/auth/official", body, "")
	if w.Code != 428 || stub.calls != 0 || len(w.Result().Cookies()) != 0 {
		t.Fatal("unaccepted signup reached upstream or created a session", w.Code, stub.calls)
	}
	if _, found, err := s.StudentBinding("001123"); err != nil || found {
		t.Fatal("unaccepted signup claimed account", err)
	}
	accepted := strings.TrimSuffix(body, "}") + `,"accept_terms":true}`
	stub.fail = true
	w = authRequest(r, "POST", "/api/auth/official", accepted, "")
	if w.Code != 401 {
		t.Fatal(w.Code, w.Body)
	}
	w = authRequest(r, "POST", "/api/auth/official", body, "")
	if w.Code != 428 || stub.calls != 1 {
		t.Fatal("failed authentication recorded consent", w.Code, stub.calls)
	}
	stub.fail = false
	w = authRequest(r, "POST", "/api/auth/official", accepted, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	cookie := w.Result().Cookies()[0]
	w = authRequest(r, "GET", "/api/auth/self", "", cookie.String())
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"consent_accepted":true`) {
		t.Fatal("consent missing from restored identity", w.Code, w.Body)
	}
	w = authRequest(r, "POST", "/api/auth/login", `{"identity":"001123","password":"school-password"}`, cookie.String())
	if w.Code != 200 {
		t.Fatal("accepted user forced to agree again", w.Code, w.Body)
	}
	w = authRequest(r, "POST", "/api/auth/official", strings.Replace(body, "001123", "002456", 1), "")
	if w.Code != 428 {
		t.Fatal("another account inherited consent", w.Code, w.Body)
	}
}

func TestStudentLoginCannotBypassConsentForLegacyUser(t *testing.T) {
	s, _ := newAuthFixture(t)
	_, _, err := s.LoginOfficial("001123", "1", "学校", "", "fixture", func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	stub := &studentLoginStub{}
	r := SetupRouter(&Handlers{PlatformAuth: s, AuthService: stub})
	w := authRequest(r, "POST", "/api/auth/login", `{"identity":"001123","password":"school-password","accept_terms":false}`, "")
	if w.Code != 428 || stub.calls != 0 {
		t.Fatal("legacy login bypassed consent", w.Code, stub.calls)
	}
}

func TestAgreementsAreReadableWithoutLogin(t *testing.T) {
	r := SetupRouter(&Handlers{})
	for path, title := range map[string]string{"/terms": "用户协议", "/privacy": "隐私政策"} {
		w := authRequest(r, "GET", path, "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<h1>"+title+"</h1>") {
			t.Fatal("agreement unavailable", path, w.Code)
		}
	}
}
