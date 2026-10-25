package service

import "errors"

// SessionCredentials keeps recoverable credentials outside the legacy password column.
// Implementations must encrypt them at rest and never expose them to browser APIs.
type SessionCredentials interface {
	SaveSessionCredentials(account, password, schoolID string) error
	SessionCredentials(account string) (password, schoolID string, err error)
}

var ErrUpstreamLoginRequired = errors.New("请登录一次以启用自动续连；之后无需反复登录")
