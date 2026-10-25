package platformauth

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestOfficialFailureLeavesNoIdentityAndSuccessStoresNoPasswordOrRawSession(t *testing.T) {
	s, _ := testStore(t)
	if _, _, e := s.LoginOfficial("001123", "1", "学校", "", "ip", func() error { return errors.New("private-password") }); e == nil || strings.Contains(e.Error(), "private-password") {
		t.Fatal("failed official verification not sanitized", e)
	}
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM platform_users").Scan(&count)
	if count != 0 {
		t.Fatal("failed verification created identity")
	}
	u, token, e := s.LoginOfficial("001123", "1", "学校", "", "ip", func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	var email, hash string
	if e = s.db.QueryRow("SELECT email,password_hash FROM platform_users WHERE id=?", u.ID).Scan(&email, &hash); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(email, token) || hash != "" || u.Email != "" {
		t.Fatal("direct login persisted credentials or exposed fake email")
	}
	if _, _, e = s.Login("001123", "school-password", "other-ip"); e == nil {
		t.Fatal("official password became a local password")
	}
	reopened, e := New(s.db, nil)
	if e != nil {
		t.Fatal(e)
	}
	current, e := reopened.Self(token)
	if e != nil || current.ID != u.ID || current.Email != "" {
		t.Fatal("direct session not restored", current, e)
	}
}
func TestConcurrentOfficialLoginReusesOneUser(t *testing.T) {
	s, _ := testStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, token, e := s.LoginOfficial("001123", "1", "学校", "", "ip", func() error { return nil })
			if e != nil {
				t.Error(e)
				return
			}
			if _, e = s.Self(token); e != nil || !s.Owns(u.ID, "001123") {
				t.Error("concurrent login missing session", e)
			}
		}()
	}
	wg.Wait()
	var users, bindings int
	s.db.QueryRow("SELECT COUNT(*) FROM platform_users").Scan(&users)
	s.db.QueryRow("SELECT COUNT(*) FROM platform_bindings").Scan(&bindings)
	if users != 1 || bindings != 1 {
		t.Fatal("duplicate concurrent official identity", users, bindings)
	}
}

func TestLegacyUnboundSchoolIsCheckedBeforeOfficialCredentialsAreWritten(t *testing.T) {
	s, _ := testStore(t)
	if _, e := s.db.Exec("CREATE TABLE users(account TEXT PRIMARY KEY,school_id REAL,token TEXT); INSERT INTO users VALUES('001123',1,'legacy-private-token')"); e != nil {
		t.Fatal(e)
	}
	called := false
	_, _, e := s.LoginOfficial("001123", "2", "其他学校", "", "ip", func() error { called = true; return nil })
	var typed *Error
	if !errors.As(e, &typed) || typed.Status != 409 || called {
		t.Fatal("unbound legacy account can be overwritten", e, called)
	}
}
