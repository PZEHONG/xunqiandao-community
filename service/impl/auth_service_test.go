package impl

import (
	"net/http"
	"testing"
	"xixunyunsign/service"
)

type officialLoginRepo struct {
	service.UserRepository
	saved           int
	password, token string
}

func (r *officialLoginRepo) SaveUser(account, password, token, latitude, longitude, phone, number, name string, school float64, sex, class, entrance, graduation string) error {
	r.saved++
	r.password = password
	r.token = token
	return nil
}
func TestOfficialLoginSendsCredentialsWithoutStoringPassword(t *testing.T) {
	r := &officialLoginRepo{}
	a := NewAuthService(r).(*AuthServiceImpl)
	a.httpClient = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.URL.Host != "api.xixunyun.com" || req.URL.Path != "/login/api" {
			t.Fatal("wrong login service")
		}
		if e := req.ParseForm(); e != nil {
			t.Fatal(e)
		}
		if req.PostForm.Get("account") != "001123" || req.PostForm.Get("password") != "school-secret" || req.PostForm.Get("school_id") != "1" {
			t.Fatal("login input lost")
		}
		return reportResponse(`{"code":20000,"data":{"token":"private-token","school_id":1}}`), nil
	})}
	token, e := a.Login("001123", "school-secret", "1")
	if e != nil || token != "private-token" || r.saved != 1 || r.password != "" || r.token != "private-token" {
		t.Fatal("password persisted or login failed", e, r.saved, r.password != "")
	}
}
func TestOfficialLoginRejectsUnusableOrWrongSchoolSession(t *testing.T) {
	for _, body := range []string{`{"code":20000,"data":{"token":"","school_id":1}}`, `{"code":20000,"data":{"token":"private-token","school_id":2}}`, `{"code":20000,"data":{"token":"private-token"}}`} {
		r := &officialLoginRepo{}
		a := NewAuthService(r).(*AuthServiceImpl)
		a.httpClient = &http.Client{Transport: reportTransport(func(*http.Request) (*http.Response, error) { return reportResponse(body), nil })}
		if _, e := a.Login("001123", "school-secret", "1"); e == nil || r.saved != 0 {
			t.Fatal("invalid official session persisted", e, r.saved)
		}
	}
}
