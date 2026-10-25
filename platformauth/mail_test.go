package platformauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSMTPConfigurationReloadAndEnvironmentPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv("AUTH_CONFIG_FILE", path)
	for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM"} {
		if _, ok := os.LookupEnv(key); ok {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
	if (SMTPMailer{}).Ready() {
		t.Fatal("unconfigured mail ready")
	}
	data := []byte("SMTP_HOST=smtp.example.com\nSMTP_PORT=465\nSMTP_USERNAME=sender@example.com\nSMTP_PASSWORD='test-only-password'\n")
	if e := os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	if !(SMTPMailer{}).Ready() {
		t.Fatal("config file not read")
	}
	t.Setenv("SMTP_HOST", "override.example.com")
	if loadSMTP().host != "override.example.com" {
		t.Fatal("environment did not override file")
	}
	t.Setenv("SMTP_PASSWORD", "")
	if (SMTPMailer{}).Ready() {
		t.Fatal("empty environment should override file")
	}
}

func TestSMTPAcceptsUserAndPassConfiguration(t *testing.T) {
	t.Setenv("AUTH_CONFIG_FILE", filepath.Join(t.TempDir(), ".env"))
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("SMTP_USER", "sender@example.com")
	t.Setenv("SMTP_PASS", "test-only-secret")
	if !(SMTPMailer{}).Ready() {
		t.Fatal("SMTP_USER / SMTP_PASS configuration not accepted")
	}
}

type failingMail struct{}

func (failingMail) Ready() bool { return true }
func (failingMail) SendCode(context.Context, string, string, string) error {
	return errors.New("private-smtp-secret")
}
func TestFailedDeliveryDoesNotCreateCodeOrExposeCredentials(t *testing.T) {
	s, m := testStore(t)
	s.mail = failingMail{}
	if e := s.SendCode(context.Background(), "a@example.com", "register", "ip"); e == nil || e.Error() != "验证码发送失败，请检查邮件服务配置后重试。" {
		t.Fatal("SMTP error not sanitized")
	}
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM platform_codes").Scan(&count)
	if count != 0 {
		t.Fatal("unsent code persisted")
	}
	s.mail = m
	if e := s.SendCode(context.Background(), "a@example.com", "register", "ip"); e != nil {
		t.Fatal("retry after failed delivery blocked", e)
	}
}
