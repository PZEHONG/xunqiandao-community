package platformauth

import (
	"context"
	"testing"
)

func TestNotificationEmailBindingLifecycle(t *testing.T) {
	s, m := testStore(t)
	u := registerTest(t, s, m, "notifyuser", "owner@example.com")
	if e := s.SendNotificationCode(context.Background(), u.ID, "notify@example.com", "1.2.3.4"); e != nil {
		t.Fatal(e)
	}
	code := m.code
	other := registerTest(t, s, m, "otheruser", "other@example.com")
	if _, e := s.BindNotificationEmail(other.ID, "notify@example.com", code); e == nil {
		t.Fatal("code must be bound to the requesting user")
	}
	n, e := s.BindNotificationEmail(u.ID, "notify@example.com", code)
	if e != nil || n.Email != "notify@example.com" || !n.Enabled || !n.Verified || n.Version != 1 {
		t.Fatalf("binding failed: %#v %v", n, e)
	}
	if _, e = s.BindNotificationEmail(u.ID, "notify@example.com", code); e == nil {
		t.Fatal("verification code was reusable")
	}
	if e = s.SendNotificationCode(context.Background(), u.ID, "changed@example.com", "1.2.3.4"); e != nil {
		t.Fatal(e)
	}
	n, e = s.BindNotificationEmail(u.ID, "changed@example.com", m.code)
	if e != nil || n.Email != "changed@example.com" || n.Version != 2 {
		t.Fatalf("email change must invalidate unsent events: %#v %v", n, e)
	}
	if n, e = s.SetNotificationEnabled(u.ID, false); e != nil || n.Enabled {
		t.Fatalf("disable failed: %#v %v", n, e)
	}
	if n, e = s.SetNotificationEnabled(u.ID, true); e != nil || !n.Enabled {
		t.Fatalf("enable failed: %#v %v", n, e)
	}
	if e = s.UnbindNotificationEmail(u.ID); e != nil {
		t.Fatal(e)
	}
	if _, found, e := s.NotificationEmail(u.ID); e != nil || found {
		t.Fatal("unbind did not remove address")
	}
}

func TestNotificationCodeNeedsWorkingMailServer(t *testing.T) {
	s, m := testStore(t)
	u := registerTest(t, s, m, "notifyuser", "owner@example.com")
	s.mail = failingMail{}
	if e := s.SendNotificationCode(context.Background(), u.ID, "notify@example.com", "1.2.3.4"); e == nil || e.Error() != "验证码发送失败，请检查邮件服务配置后重试。" {
		t.Fatal("SMTP failure not sanitized")
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM platform_notification_codes WHERE user_id=?`, u.ID).Scan(&count)
	if count != 0 {
		t.Fatal("undelivered notification code persisted")
	}
}
