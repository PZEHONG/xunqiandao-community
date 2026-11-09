package platformauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type captchaTransport func(*http.Request) (*http.Response, error)

func (f captchaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func captchaSettings(t *testing.T) {
	t.Helper()
	t.Setenv("GEETEST_ENABLED", "true")
	t.Setenv("GEETEST_CAPTCHA_ID", "public-id")
	t.Setenv("GEETEST_CAPTCHA_KEY", "private-key")
}
func proofFixture() CaptchaProof { return CaptchaProof{"lot-123", "output", "pass", "1720000000"} }
func TestGeeTestSignedServerValidation(t *testing.T) {
	captchaSettings(t)
	called := false
	g := GeeTest{Client: &http.Client{Transport: captchaTransport(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.URL.String() != "https://gcaptcha4.geetest.com/validate?captcha_id=public-id" || r.Method != "POST" {
			t.Fatal("wrong verification request", r.URL)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatal("wrong content type")
		}
		if e := r.ParseForm(); e != nil {
			t.Fatal(e)
		}
		mac := hmac.New(sha256.New, []byte("private-key"))
		mac.Write([]byte("lot-123"))
		if r.PostForm.Get("sign_token") != hex.EncodeToString(mac.Sum(nil)) || r.PostForm.Get("pass_token") != "pass" || r.PostForm.Get("captcha_output") != "output" || r.PostForm.Get("gen_time") != "1720000000" {
			t.Fatal("incorrect signed proof")
		}
		if d, ok := r.Context().Deadline(); !ok || time.Until(d) > 5*time.Second {
			t.Fatal("unbounded verification")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"success","result":"success"}`))}, nil
	})}}
	if e := g.verify(context.Background(), g.Config(), proofFixture()); e != nil || !called {
		t.Fatal(e)
	}
}
func TestGeeTestRejectsFailuresWithoutMail(t *testing.T) {
	captchaSettings(t)
	for _, tc := range []struct {
		name, body     string
		status         int
		transportError bool
		want           int
	}{
		{"failed", `{"status":"success","result":"fail","reason":"private-key"}`, 200, false, 400},
		{"error with success", `{"status":"error","result":"success"}`, 200, false, 400},
		{"invalid JSON", `{`, 200, false, 503},
		{"HTTP error", `{"status":"success","result":"success"}`, 502, false, 503},
		{"network error", ``, 0, true, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m := testStore(t)
			s.Captcha.Client = &http.Client{Transport: captchaTransport(func(*http.Request) (*http.Response, error) {
				if tc.transportError {
					return nil, errors.New("private-key")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			e := s.SendVerifiedCode(context.Background(), "s@example.com", "register", "ip", proofFixture())
			var typed *Error
			if !errors.As(e, &typed) || typed.Status != tc.want || strings.Contains(e.Error(), "private-key") || m.code != "" {
				t.Fatalf("failure not closed: %v", e)
			}
		})
	}
}
func TestVerifiedProofCannotSendAnotherEmail(t *testing.T) {
	captchaSettings(t)
	s, m := testStore(t)
	s.Captcha.Client = &http.Client{Transport: captchaTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"success","result":"success"}`))}, nil
	})}
	if e := s.SendVerifiedCode(context.Background(), "a@example.com", "register", "ip", proofFixture()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Register("001123", "a@example.com", "Correct-password-9", m.code); e != nil {
		t.Fatal("verified email not usable", e)
	}
	m.code = ""
	reopened, e := New(s.db, m)
	if e != nil {
		t.Fatal(e)
	}
	reopened.Captcha = s.Captcha
	if e = reopened.SendVerifiedCode(context.Background(), "b@example.com", "register", "ip", proofFixture()); e == nil || m.code != "" {
		t.Fatal("proof replay sent mail", e)
	}
}
func TestNotificationCaptchaProofIsRequiredAndSingleUse(t *testing.T) {
	captchaSettings(t)
	s, m := testStore(t)
	u := registerTest(t, s, m, "notifycaptcha", "owner@example.com")
	m.code = ""
	s.Captcha.Client = &http.Client{Transport: captchaTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"success","result":"success"}`))}, nil
	})}
	if e := s.SendVerifiedNotificationCode(context.Background(), u.ID, "notify@example.com", "ip", CaptchaProof{}); e == nil || m.code != "" {
		t.Fatal("missing proof allowed mail", e)
	}
	if e := s.SendVerifiedNotificationCode(context.Background(), u.ID, "notify@example.com", "ip", proofFixture()); e != nil || m.code == "" {
		t.Fatal("verified send failed", e)
	}
	m.code = ""
	if e := s.SendVerifiedNotificationCode(context.Background(), u.ID, "another@example.com", "ip", proofFixture()); e == nil || m.code != "" {
		t.Fatal("reused proof allowed mail", e)
	}
}
func TestCaptchaConfigurationReloadAndNoKeyDisclosure(t *testing.T) {
	t.Setenv("GEETEST_API_KEY", "")
	t.Setenv("AUTH_CONFIG_FILE", filepath.Join(t.TempDir(), ".env"))
	for _, key := range []string{"GEETEST_ENABLED", "GEETEST_CAPTCHA_ID", "GEETEST_CAPTCHA_KEY"} {
		if _, ok := os.LookupEnv(key); ok {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
	g := GeeTest{}
	if c := g.Config(); !c.Enabled || c.Ready {
		t.Fatal("default must require configuration")
	}
	if e := os.WriteFile(os.Getenv("AUTH_CONFIG_FILE"), []byte("GEETEST_CAPTCHA_ID=public\nGEETEST_CAPTCHA_KEY=secret\n"), 0600); e != nil {
		t.Fatal(e)
	}
	c := g.Config()
	b, _ := json.Marshal(c)
	if !c.Ready || strings.Contains(string(b), "secret") {
		t.Fatal("config or disclosure", string(b))
	}
	t.Setenv("GEETEST_CAPTCHA_KEY", "")
	if g.Config().Ready {
		t.Fatal("empty env must override file")
	}
}

func TestGeeTestAcceptsAPIKeyConfiguration(t *testing.T) {
	t.Setenv("AUTH_CONFIG_FILE", filepath.Join(t.TempDir(), ".env"))
	t.Setenv("GEETEST_ENABLED", "true")
	t.Setenv("GEETEST_CAPTCHA_ID", "public")
	t.Setenv("GEETEST_CAPTCHA_KEY", "")
	t.Setenv("GEETEST_API_KEY", "private")
	if !(GeeTest{}).Config().Ready {
		t.Fatal("GEETEST_API_KEY configuration not accepted")
	}
}
func TestCaptchaMissingProofAndRateLimit(t *testing.T) {
	captchaSettings(t)
	s, m := testStore(t)
	for i := 0; i < 21; i++ {
		e := s.SendVerifiedCode(context.Background(), "a@example.com", "register", "ip", CaptchaProof{})
		var typed *Error
		want := 400
		if i == 20 {
			want = 429
		}
		if !errors.As(e, &typed) || typed.Status != want || m.code != "" {
			t.Fatal("missing proof or throttle failed", i, e)
		}
	}
	t.Setenv("GEETEST_CAPTCHA_KEY", "")
	if e := s.SendVerifiedCode(context.Background(), "a@example.com", "register", "another-ip", proofFixture()); e == nil {
		t.Fatal("unconfigured validation allowed")
	}
}
