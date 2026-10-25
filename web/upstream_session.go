package web

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"xixunyunsign/service"
)

// Keep the local session valid when only the school's credential needs updating.
func upstreamSessionFailure(c *gin.Context, err error) bool {
	if !errors.Is(err, service.ErrUpstreamLoginRequired) {
		return false
	}
	c.JSON(http.StatusUnauthorized, gin.H{"code": "UPSTREAM_LOGIN_REQUIRED", "error": service.ErrUpstreamLoginRequired.Error()})
	return true
}
