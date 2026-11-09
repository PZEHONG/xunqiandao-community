package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"xixunyunsign/platformauth"

	"github.com/gin-gonic/gin"
)

const authCookie = "xun_session"

func authError(c *gin.Context, e error) {
	var typed *platformauth.Error
	if errors.As(e, &typed) {
		c.JSON(typed.Status, gin.H{"error": typed.Message})
		return
	}
	c.JSON(503, gin.H{"error": "账号服务暂时不可用，请稍后重试。"})
}
func sameOrigin(c *gin.Context) bool {
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := c.GetHeader("Origin"); origin != "" {
		u, e := url.Parse(origin)
		scheme := "http"
		if requestIsHTTPS(c) {
			// TLS 由反向代理终止时 c.Request.TLS 为空，仅信任上游代理覆写的 X-Forwarded-Proto。
			scheme = "https"
		}
		if e != nil || u.Scheme != scheme || u.Host != c.Request.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return false
		}
	}
	return true
}
func currentPlatformUser(c *gin.Context) platformauth.User {
	value, _ := c.Get("platform-user")
	u, _ := value.(platformauth.User)
	return u
}
func (h *Handlers) protectAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if (!strings.HasPrefix(path, "/api/") && path != "/_AMapService" && !strings.HasPrefix(path, "/_AMapService/")) || path == "/api/health" {
			c.Next()
			return
		}
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" && !sameOrigin(c) {
			c.AbortWithStatusJSON(403, gin.H{"error": "请从讯签到页面提交操作。"})
			return
		}
		if h.PlatformAuth == nil {
			c.AbortWithStatusJSON(503, gin.H{"error": "账号服务尚未就绪，请重启服务。"})
			return
		}
		switch path {
		case "/api/auth/status", "/api/auth/code", "/api/auth/register", "/api/auth/login", "/api/auth/reset", "/api/auth/official", "/api/auth/schools":
			c.Next()
			return
		}
		token, _ := c.Cookie(authCookie)
		user, e := h.PlatformAuth.Self(token)
		if e != nil {
			var typed *platformauth.Error
			if errors.As(e, &typed) && typed.Status == 401 {
				c.AbortWithStatusJSON(401, gin.H{"error": typed.Message, "code": "AUTH_REQUIRED"})
			} else {
				c.AbortWithStatusJSON(503, gin.H{"error": "账号服务暂时不可用。"})
			}
			return
		}
		c.Set("platform-user", user)
		if path == "/api/login" {
			c.Next()
			return
		} // The handler verifies credentials before claiming a binding.
		// All account-bearing business requests pass the same ownership check.
		account := c.Query("account")
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" && path != "/api/auth/logout" {
			mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
			if mediaType == "multipart/form-data" {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11<<20)
				if e = c.Request.ParseMultipartForm(1 << 20); e != nil {
					c.AbortWithStatusJSON(400, gin.H{"error": "附件格式错误或超过 10 MB。"})
					return
				}
				if c.Request.MultipartForm != nil {
					defer c.Request.MultipartForm.RemoveAll()
				}
				account = c.PostForm("account")
			} else {
				limit := int64(64 << 10)
				if path == "/api/reports" {
					limit = 1 << 20
				}
				body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
				if err != nil {
					c.AbortWithStatusJSON(400, gin.H{"error": "请求内容超过限制。"})
					return
				}
				var input struct {
					Account string `json:"account"`
				}
				if len(body) > 0 && json.Unmarshal(body, &input) != nil {
					c.AbortWithStatusJSON(400, gin.H{"error": "请求格式有误。"})
					return
				}
				account = input.Account
				c.Request.Body = io.NopCloser(bytes.NewReader(body))
			}
		}
		if account != "" && !h.PlatformAuth.Owns(user.ID, account) {
			c.AbortWithStatusJSON(403, gin.H{"error": "请先连接属于当前用户的习讯云账号。"})
			return
		}
		// No account-less route may reach a service that accepts an account.
		if account == "" && (path == "/api/query" || path == "/api/sign" || strings.HasPrefix(path, "/api/sign/") || strings.HasPrefix(path, "/api/reports") || strings.HasPrefix(path, "/api/auto-sign/")) {
			c.AbortWithStatusJSON(400, gin.H{"error": "请先连接习讯云账号。"})
			return
		}
		c.Next()
	}
}
func authBody(c *gin.Context, input any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(input) != nil {
		c.JSON(400, gin.H{"error": "请完整填写表单。"})
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		c.JSON(400, gin.H{"error": "请求格式有误。"})
		return false
	}
	return true
}
func setAuthCookie(c *gin.Context, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: authCookie, Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: secureSessionCookie(c), SameSite: http.SameSiteLaxMode})
}
func registerAuthRoutes(router *gin.Engine, h *Handlers) {
	group := router.Group("/api/auth")
	registerOfficialAuthRoutes(group, h)
	group.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"registration": false, "email_login": false, "official_login": true, "session_days": 365, "agreement_version": platformauth.AgreementVersion})
	})
	for _, path := range []string{"/code", "/register", "/reset"} {
		group.POST(path, func(c *gin.Context) {
			c.JSON(410, gin.H{"error": "已取消邮箱注册和找回密码，请使用习讯云登录；首次验证成功后会自动创建账号。"})
		})
	}
	group.POST("/login", func(c *gin.Context) {
		var input struct {
			Identity    string `json:"identity"`
			Password    string `json:"password"`
			AcceptTerms bool   `json:"accept_terms"`
		}
		if !authBody(c, &input) {
			return
		}
		input.Identity = strings.TrimSpace(input.Identity)
		if !h.requireConsent(c, input.Identity, input.AcceptTerms) {
			return
		}
		if input.Password == "" || len(input.Password) > 256 {
			c.JSON(401, gin.H{"error": "请输入学号和密码。"})
			return
		}
		binding, found, e := h.PlatformAuth.StudentBinding(input.Identity)
		if e != nil {
			authError(c, e)
			return
		}
		var user platformauth.User
		var token string
		if found {
			if h.AuthService == nil {
				c.JSON(503, gin.H{"error": "习讯云连接服务暂时不可用。"})
				return
			}
			old, _ := c.Cookie(authCookie)
			user, token, e = h.PlatformAuth.LoginOfficialContext(c.Request.Context(), binding.Account, binding.SchoolID, binding.SchoolName, old, requestClientIP(c), func(ctx context.Context) error {
				return h.verifyOfficialContext(ctx, binding.Account, input.Password, binding.SchoolID)
			})
		} else {
			// Preserve existing local accounts; new users enter through official verification.
			user, token, e = h.PlatformAuth.Login(input.Identity, input.Password, requestClientIP(c))
		}
		if e != nil {
			authError(c, e)
			return
		}
		// Replace the browser's previous session instead of leaving it active.
		if !h.saveConsent(c, user.ID, token, input.AcceptTerms) {
			return
		}
		if old, _ := c.Cookie(authCookie); old != "" {
			if e = h.PlatformAuth.Logout(old); e != nil {
				h.PlatformAuth.Logout(token)
				authError(c, e)
				return
			}
		}
		setAuthCookie(c, token, int(platformauth.SessionAge.Seconds()))
		c.JSON(200, gin.H{"user": user, "message": "登录成功。", "consent_accepted": true, "agreement_version": platformauth.AgreementVersion})
	})
	group.GET("/self", func(c *gin.Context) {
		user := currentPlatformUser(c)
		bindings, e := h.PlatformAuth.Bindings(user.ID)
		if e != nil {
			authError(c, e)
			return
		}
		accepted, e := h.PlatformAuth.HasConsent(user.ID)
		if e != nil {
			authError(c, e)
			return
		}
		notificationEmail, found, e := h.PlatformAuth.NotificationEmail(user.ID)
		if e != nil {
			authError(c, e)
			return
		}
		var notification any
		if found {
			notification = notificationEmail
		}
		// Slow business responses must not overwrite a newer login cookie.
		// The page checks this short local endpoint hourly and on visibility.
		token, _ := c.Cookie(authCookie)
		setAuthCookie(c, token, int(platformauth.SessionAge.Seconds()))
		c.JSON(200, gin.H{"user": user, "bindings": bindings, "consent_accepted": accepted, "agreement_version": platformauth.AgreementVersion, "notification_email": notification})
	})
	group.GET("/notification-email", func(c *gin.Context) {
		user := currentPlatformUser(c)
		value, found, e := h.PlatformAuth.NotificationEmail(user.ID)
		if e != nil {
			authError(c, e)
			return
		}
		if !found {
			c.JSON(200, gin.H{"message": "尚未绑定签到提醒邮箱。"})
			return
		}
		c.JSON(200, value)
	})
	group.GET("/notification-email/captcha", func(c *gin.Context) {
		c.JSON(200, h.PlatformAuth.Captcha.Config())
	})
	group.POST("/notification-email/code", func(c *gin.Context) {
		var input struct {
			Email   string                    `json:"email"`
			Captcha platformauth.CaptchaProof `json:"captcha"`
		}
		if !authBody(c, &input) {
			return
		}
		if e := h.PlatformAuth.SendVerifiedNotificationCode(c.Request.Context(), currentPlatformUser(c).ID, input.Email, requestClientIP(c), input.Captcha); e != nil {
			authError(c, e)
			return
		}
		c.JSON(200, gin.H{"message": "验证码已发送，请查收邮箱。"})
	})
	group.POST("/notification-email/bind", func(c *gin.Context) {
		var input struct {
			Email string `json:"email"`
			Code  string `json:"code"`
		}
		if !authBody(c, &input) {
			return
		}
		value, e := h.PlatformAuth.BindNotificationEmail(currentPlatformUser(c).ID, input.Email, input.Code)
		if e != nil {
			authError(c, e)
			return
		}
		c.JSON(200, gin.H{"notification_email": value, "message": "签到提醒邮箱已绑定。"})
	})
	group.POST("/notification-email/unbind", func(c *gin.Context) {
		if e := h.PlatformAuth.UnbindNotificationEmail(currentPlatformUser(c).ID); e != nil {
			authError(c, e)
			return
		}
		c.JSON(200, gin.H{"message": "已解除签到提醒邮箱绑定。"})
	})
	group.POST("/notification-email/enabled", func(c *gin.Context) {
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		if !authBody(c, &input) {
			return
		}
		if input.Enabled == nil {
			c.JSON(400, gin.H{"error": "请选择是否接收提醒。"})
			return
		}
		value, e := h.PlatformAuth.SetNotificationEnabled(currentPlatformUser(c).ID, *input.Enabled)
		if e != nil {
			authError(c, e)
			return
		}
		c.JSON(200, gin.H{"notification_email": value, "message": "提醒设置已更新。"})
	})
	group.POST("/logout", func(c *gin.Context) {
		token, _ := c.Cookie(authCookie)
		if e := h.PlatformAuth.Logout(token); e != nil {
			authError(c, e)
			return
		}
		setAuthCookie(c, "", -1)
		c.JSON(200, gin.H{"message": "已退出登录。"})
	})
}
