package web

import (
	"context"
	"errors"
	"strings"
	"xixunyunsign/platformauth"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) verifyOfficial(account, password, schoolID string) error {
	return h.verifyOfficialContext(context.Background(), account, password, schoolID)
}

func (h *Handlers) verifyOfficialContext(ctx context.Context, account, password, schoolID string) error {
	if h.AuthService == nil {
		return errors.New("upstream unavailable")
	}
	token, e := h.loginWithContext(ctx, account, password, schoolID)
	if e == nil && token == "" {
		return errors.New("missing upstream session")
	}
	return e
}

func registerOfficialAuthRoutes(group *gin.RouterGroup, h *Handlers) {
	group.GET("/schools", func(c *gin.Context) {
		name := strings.TrimSpace(c.Query("school_name"))
		if len([]rune(name)) < 2 || len(name) > 256 {
			c.JSON(400, gin.H{"error": "请输入至少 2 个字的学校名称。"})
			return
		}
		if h.QueryService == nil {
			c.JSON(503, gin.H{"error": "学校搜索服务暂时不可用。"})
			return
		}
		if e := h.PlatformAuth.AllowOfficialSchoolSearch(requestClientIP(c)); e != nil {
			authError(c, e)
			return
		}
		schools, e := h.QueryService.SearchSchool(name)
		if e != nil {
			c.JSON(503, gin.H{"error": "学校搜索失败，请稍后重试。"})
			return
		}
		if len(schools) > 50 {
			schools = schools[:50]
		}
		c.JSON(200, gin.H{"schools": schools})
	})
	group.POST("/official", func(c *gin.Context) {
		var input struct {
			Account     string `json:"account"`
			Password    string `json:"password"`
			SchoolID    string `json:"school_id"`
			SchoolName  string `json:"school_name"`
			AcceptTerms bool   `json:"accept_terms"`
		}
		if !authBody(c, &input) {
			return
		}
		if !h.requireConsent(c, input.Account, input.AcceptTerms) {
			return
		}
		if input.Password == "" || len(input.Password) > 256 {
			c.JSON(400, gin.H{"error": "请填写习讯云密码。"})
			return
		}
		if h.AuthService == nil {
			c.JSON(503, gin.H{"error": "习讯云连接服务暂时不可用。"})
			return
		}
		old, _ := c.Cookie(authCookie)
		user, token, e := h.PlatformAuth.LoginOfficialContext(c.Request.Context(), input.Account, input.SchoolID, input.SchoolName, old, requestClientIP(c), func(ctx context.Context) error {
			return h.verifyOfficialContext(ctx, input.Account, input.Password, input.SchoolID)
		})
		if e != nil {
			authError(c, e)
			return
		}
		if !h.saveConsent(c, user.ID, token, input.AcceptTerms) {
			return
		}
		setAuthCookie(c, token, int(platformauth.SessionAge.Seconds()))
		c.JSON(200, gin.H{"user": user, "message": "习讯云登录成功。", "consent_accepted": true, "agreement_version": platformauth.AgreementVersion})
	})
}
