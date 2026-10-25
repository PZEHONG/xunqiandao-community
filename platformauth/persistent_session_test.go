package platformauth

import (
	"testing"
	"time"
)

func TestSessionRenewsForOneYearAndNeverRevivesExpiredOrRevokedTokens(t *testing.T) {
	s, _ := testStore(t)
	now := time.Now().Truncate(time.Second)
	s.now = func() time.Time { return now }
	_, token, err := s.LoginOfficial("001123", "1", "学校", "", "ip", func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	start := now
	for _, days := range []int{30, 360, 700} {
		now = start.Add(time.Duration(days) * 24 * time.Hour)
		if _, err = s.Self(token); err != nil {
			t.Fatal("active session expired", days, err)
		}
		var expiry int64
		if err = s.db.QueryRow("SELECT expires_at FROM platform_sessions WHERE token_hash=?", digest(token)).Scan(&expiry); err != nil {
			t.Fatal(err)
		}
		if expiry != now.Add(365*24*time.Hour).Unix() {
			t.Fatal("session not extended for one year", days)
		}
	}
	if err = s.Logout(token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Self(token); err == nil {
		t.Fatal("revoked token revived")
	}
	_, token, err = s.LoginOfficial("001123", "1", "学校", "", "ip", func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(366 * 24 * time.Hour)
	if _, err = s.Self(token); err == nil {
		t.Fatal("expired token revived")
	}
}
