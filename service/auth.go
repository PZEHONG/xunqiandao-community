// service/auth.go
package service

import "context"

// AuthService retains compatibility with existing integrations.
type AuthService interface {
	Login(account, password, schoolID string) (string, error)
}

// ContextAuthService is optional; web requests use it when available.
type ContextAuthService interface {
	LoginContext(context.Context, string, string, string) (string, error)
}

type loginCommitGuardKey struct{}
type LoginCommitGuard func(context.Context, func() error) error

// WithLoginCommitGuard permits the owner to recheck authorization immediately
// before saving upstream credentials under its administrative state lock.
func WithLoginCommitGuard(ctx context.Context, guard LoginCommitGuard) context.Context {
	return context.WithValue(ctx, loginCommitGuardKey{}, guard)
}

func CommitLogin(ctx context.Context, save func() error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if guard, ok := ctx.Value(loginCommitGuardKey{}).(LoginCommitGuard); ok {
		return guard(ctx, save)
	}
	return save()
}
