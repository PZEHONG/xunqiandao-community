package web

import (
	"github.com/gin-gonic/gin"
	"xixunyunsign/platformauth"
)

func (h *Handlers) requireConsent(c *gin.Context, account string, confirmed bool) bool {
	if confirmed {
		return true
	}
	accepted, err := h.PlatformAuth.HasAccountConsent(account)
	if err != nil {
		authError(c, err)
		return false
	}
	if !accepted {
		c.JSON(428, gin.H{"error": "请先阅读并勾选同意《用户协议》和《隐私政策》。", "code": "CONSENT_REQUIRED", "agreement_version": platformauth.AgreementVersion})
	}
	return accepted
}

func (h *Handlers) saveConsent(c *gin.Context, userID int64, token string, confirmed bool) bool {
	if !confirmed {
		return true
	}
	if err := h.PlatformAuth.RecordConsent(userID); err != nil {
		_ = h.PlatformAuth.Logout(token)
		authError(c, err)
		return false
	}
	return true
}
