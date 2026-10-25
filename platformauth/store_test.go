package platformauth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type testMail struct {
	code  string
	ready bool
}

func (m *testMail) Ready() bool                                         { return m.ready }
func (m *testMail) SendCode(_ context.Context, _, code, _ string) error { m.code = code; return nil }
func testStore(t *testing.T) (*Store, *testMail) {
	t.Helper()
	db, e := sql.Open("sqlite3", filepath.Join(t.TempDir(), "auth.db")+"?_busy_timeout=5000")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	m := &testMail{ready: true}
	s, e := New(db, m)
	if e != nil {
		t.Fatal(e)
	}
	return s, m
}
func registerTest(t *testing.T, s *Store, m *testMail, name, email string) User {
	t.Helper()
	if e := s.SendCode(context.Background(), email, "register", email); e != nil {
		t.Fatal(e)
	}
	u, e := s.Register(name, email, "Correct-password-9", m.code)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
func TestRegistrationAndPersistentSession(t *testing.T) {
	s, m := testStore(t)
	u := registerTest(t, s, m, "alice", "Alice@example.com")
	if u.Email != "alice@example.com" {
		t.Fatal(u)
	}
	var hash string
	s.db.QueryRow("SELECT password_hash FROM platform_users WHERE id=?", u.ID).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("Correct-password-9")) != nil {
		t.Fatal("password not hashed")
	}
	_, token, e := s.Login("ALICE", "Correct-password-9", "ip")
	if e != nil {
		t.Fatal(e)
	}
	if len(token) < 40 {
		t.Fatal("short token")
	}
	reopened, e := New(s.db, m)
	if e != nil {
		t.Fatal(e)
	}
	actual, e := reopened.Self(token)
	if e != nil || actual.ID != u.ID {
		t.Fatal(actual, e)
	}
	var stored string
	s.db.QueryRow("SELECT token_hash FROM platform_sessions").Scan(&stored)
	if stored == token || strings.Contains(stored, token) {
		t.Fatal("raw session stored")
	}
	if e := s.Logout(token); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Self(token); e == nil {
		t.Fatal("logout did not revoke")
	}
}
func TestVerificationValidationAndRateLimits(t *testing.T) {
	s, m := testStore(t)
	ctx := context.Background()
	if e := s.SendCode(ctx, "bad", "register", "ip"); e == nil {
		t.Fatal("invalid email accepted")
	}
	if e := s.SendCode(ctx, "a@example.com", "register", "ip"); e != nil {
		t.Fatal(e)
	}
	code := m.code
	if e := s.SendCode(ctx, "a@example.com", "register", "ip"); e == nil {
		t.Fatal("cooldown bypass")
	}
	if _, e := s.Register("alice", "a@example.com", "short", code); e == nil {
		t.Fatal("weak password")
	}
	for i := 0; i < 5; i++ {
		if _, e := s.Register("alice", "a@example.com", "Valid-password-9", "wrong!"); e == nil {
			t.Fatal("wrong code accepted")
		}
	}
	if _, e := s.Register("alice", "a@example.com", "Valid-password-9", code); e == nil {
		t.Fatal("attempt limit bypass")
	}
	s.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if _, e := s.Register("alice", "a@example.com", "Valid-password-9", code); e == nil {
		t.Fatal("expired code accepted")
	}
	m.ready = false
	if e := s.SendCode(ctx, "b@example.com", "register", "ip"); e == nil {
		t.Fatal("unconfigured mail succeeds")
	}
}
func TestCodeSingleUseAndDuplicateRegistration(t *testing.T) {
	s, m := testStore(t)
	registerTest(t, s, m, "alice", "alice@example.com")
	if _, e := s.Register("bob", "alice@example.com", "Correct-password-9", m.code); e == nil {
		t.Fatal("code reused")
	}
	if e := s.SendCode(context.Background(), "bob@example.com", "register", "other"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Register("ALICE", "bob@example.com", "Correct-password-9", m.code); e == nil {
		t.Fatal("duplicate username")
	}
}
func TestResetRevokesAllSessionsAndPurposeIsolated(t *testing.T) {
	s, m := testStore(t)
	registerTest(t, s, m, "alice", "alice@example.com")
	_, one, _ := s.Login("alice", "Correct-password-9", "ip1")
	_, two, _ := s.Login("alice", "Correct-password-9", "ip2")
	if e := s.SendCode(context.Background(), "alice@example.com", "reset", "ip"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Register("bob", "alice@example.com", "Correct-password-9", m.code); e == nil {
		t.Fatal("reset code used to register")
	}
	if e := s.Reset("alice@example.com", m.code, "New-password-9"); e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{one, two} {
		if _, e := s.Self(token); e == nil {
			t.Fatal("old session survived reset")
		}
	}
	if _, _, e := s.Login("alice", "Correct-password-9", "ip3"); e == nil {
		t.Fatal("old password works")
	}
	if _, _, e := s.Login("alice", "New-password-9", "ip3"); e != nil {
		t.Fatal(e)
	}
}
func TestOwnershipAndExpiry(t *testing.T) {
	s, m := testStore(t)
	alice := registerTest(t, s, m, "alice", "a@example.com")
	bob := registerTest(t, s, m, "bobby", "b@example.com")
	if e := s.Bind(alice.ID, "upstream", "1", "学校"); e != nil {
		t.Fatal(e)
	}
	if e := s.Bind(bob.ID, "upstream", "1", "学校"); e == nil {
		t.Fatal("ownership stolen")
	}
	if s.Owns(bob.ID, "upstream") || !s.Owns(alice.ID, "upstream") {
		t.Fatal("ownership wrong")
	}
	_, token, _ := s.Login("alice", "Correct-password-9", "ip")
	s.now = func() time.Time { return time.Now().Add(SessionAge + time.Hour) }
	if _, e := s.Self(token); e == nil {
		t.Fatal("expired session valid")
	}
}

func TestResetWithSingleDatabaseConnection(t *testing.T) {
	s, m := testStore(t)
	registerTest(t, s, m, "alice", "a@example.com")
	if e := s.SendCode(context.Background(), "a@example.com", "reset", "ip"); e != nil {
		t.Fatal(e)
	}
	s.db.SetMaxOpenConns(1)
	done := make(chan error, 1)
	go func() { done <- s.Reset("a@example.com", m.code, "New-password-9") }()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	// Race instrumentation also slows the bcrypt work inside Reset.
	case <-time.After(15 * time.Second):
		s.db.SetMaxOpenConns(2)
		s.db.Ping()
		<-done
		t.Fatal("password reset deadlocked with one connection")
	}
}
func TestConcurrentRegistrationConsumesCodeOnce(t *testing.T) {
	s, m := testStore(t)
	if e := s.SendCode(context.Background(), "a@example.com", "register", "ip"); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	code := m.code
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Register("alice", "a@example.com", "Correct-password-9", code)
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for e := range results {
		if e == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("single use violated", successes)
	}
}
func TestCodeIPLimitAndLoginLimit(t *testing.T) {
	s, _ := testStore(t)
	for i := 0; i < 5; i++ {
		if e := s.SendCode(context.Background(), fmt.Sprintf("u%d@example.com", i), "register", "same-ip"); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.SendCode(context.Background(), "last@example.com", "register", "same-ip"); e == nil {
		t.Fatal("mail IP limit bypass")
	}
	for i := 0; i < 15; i++ {
		_, _, e := s.Login("unknown", "Wrong-password-9", "login-ip")
		var typed *Error
		if !errors.As(e, &typed) || typed.Status != 401 {
			t.Fatal("unexpected login failure", e)
		}
	}
	_, _, e := s.Login("unknown", "Wrong-password-9", "login-ip")
	var typed *Error
	if !errors.As(e, &typed) || typed.Status != 429 {
		t.Fatal("login limit bypass", e)
	}
}
