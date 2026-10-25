package web

import (
	"context"
	"xixunyunsign/service"
)

// loginWithContext preserves old AuthService stubs while using cancellation in
// production implementations. The guard only wraps local credential writes.
func (h *Handlers) loginWithContext(ctx context.Context, account, password, schoolID string) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if h.PlatformAuth != nil {
		ctx = service.WithLoginCommitGuard(ctx, h.PlatformAuth.LoginCommitGuard(account, schoolID))
	}
	if auth, ok := h.AuthService.(service.ContextAuthService); ok {
		return auth.LoginContext(ctx, account, password, schoolID)
	}
	token, e := h.AuthService.Login(account, password, schoolID)
	if e == nil {
		e = ctx.Err()
	}
	return token, e
}
