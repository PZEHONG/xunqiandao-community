package impl

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
	"xixunyunsign/service"
)

func TestLoginContextCancellationDoesNotPersistCredentials(t *testing.T) {
	repo := &officialLoginRepo{}
	auth := NewAuthService(repo).(*AuthServiceImpl)
	contextual, ok := interface{}(auth).(service.ContextAuthService)
	if !ok {
		t.Fatal("auth service does not propagate request cancellation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	auth.httpClient = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
		cancel()
		return reportResponse(`{"code":20000,"data":{"token":"token","school_id":1}}`), nil
	})}
	if _, e := contextual.LoginContext(ctx, "cancel001", "password", "1"); !errors.Is(e, context.Canceled) || repo.saved != 0 {
		t.Fatal("cancelled login persisted credentials", e, repo.saved)
	}
}
func TestLoginContextCanCancelWhileWaitingForRenewal(t *testing.T) {
	auth := NewAuthService(&officialLoginRepo{})
	contextual, ok := auth.(service.ContextAuthService)
	if !ok {
		t.Fatal("missing context-aware login")
	}
	state := accountRenewalState("locked001")
	state.mu.Lock()
	defer state.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := contextual.LoginContext(ctx, "locked001", "password", "1"); done <- e }()
	select {
	case e := <-done:
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Fatal(e)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancelled login still waits for account renewal lock")
	}
}
func TestLoginContextCommitGuardRejectsPersistence(t *testing.T) {
	repo := &officialLoginRepo{}
	auth := NewAuthService(repo).(*AuthServiceImpl)
	contextual, ok := interface{}(auth).(service.ContextAuthService)
	if !ok {
		t.Fatal("missing guarded login")
	}
	auth.httpClient = &http.Client{Transport: reportTransport(func(*http.Request) (*http.Response, error) {
		return reportResponse(`{"code":20000,"data":{"token":"token","school_id":1}}`), nil
	})}
	denied := errors.New("disabled")
	ctx := service.WithLoginCommitGuard(context.Background(), func(context.Context, func() error) error { return denied })
	if _, e := contextual.LoginContext(ctx, "guard001", "password", "1"); !errors.Is(e, denied) || repo.saved != 0 {
		t.Fatal("guard bypassed", e, repo.saved)
	}
}
