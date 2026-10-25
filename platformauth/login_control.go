package platformauth

import (
	"context"
	"strings"
	"sync"
	"time"
)

const LoginTimeout = 30 * time.Second

type accountLoginGate struct {
	token chan struct{}
	refs  int
}

// LockLogin bounds queued work and serializes only the same upstream account.
// The release function must be called even when verification fails.
func (s *Store) LockLogin(ctx context.Context, account string) (func(), error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	key := strings.ToLower(account)
	s.loginMu.Lock()
	if s.loginGates == nil {
		s.loginGates = make(map[string]*accountLoginGate)
	}
	gate := s.loginGates[key]
	if gate == nil {
		if len(s.loginGates) >= 1024 {
			s.loginMu.Unlock()
			return nil, failure(429, "Too many login requests. Please retry later.")
		}
		gate = &accountLoginGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		s.loginGates[key] = gate
	}
	if gate.refs >= 9 {
		s.loginMu.Unlock()
		return nil, failure(429, "Too many pending logins for this account.")
	}
	gate.refs++
	s.loginMu.Unlock()
	drop := func() {
		s.loginMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(s.loginGates, key)
		}
		s.loginMu.Unlock()
	}
	select {
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	case <-gate.token:
		if e := ctx.Err(); e != nil {
			gate.token <- struct{}{}
			drop()
			return nil, e
		}
		var once sync.Once
		return func() { once.Do(func() { gate.token <- struct{}{}; drop() }) }, nil
	}
}

// AllowLogin shares one budget across official login and authenticated reconnect.
func (s *Store) AllowLogin(account, ip string) error {
	if e := s.limit("upstream-login-ip:"+ip, 30, 15*time.Minute); e != nil {
		return e
	}
	return s.limit("upstream-login-user:"+strings.ToLower(account), 15, 15*time.Minute)
}

// LoginCommitGuard serializes local credential persistence with binding checks.
func (s *Store) LoginCommitGuard(account, schoolID string) func(context.Context, func() error) error {
	return func(ctx context.Context, save func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := s.CheckAccountSchool(account, schoolID); e != nil {
			return e
		}
		return save()
	}
}
