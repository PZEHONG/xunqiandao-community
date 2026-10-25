// web/handlers.go
package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"xixunyunsign/autosign"
	"xixunyunsign/platformauth"
	// "xixunyunsign/cmd" // No longer calling cmd directly
	"xixunyunsign/service" // Depend on service interfaces
	// "xixunyunsign/utils" // No longer needed directly if SchoolInfo is in service

	"github.com/gin-gonic/gin"
)

// Handlers holds dependencies for web handlers, like services.
type Handlers struct {
	PlatformAuth  *platformauth.Store
	AutoSign      *autosign.Store
	AuthService   service.AuthService
	QueryService  service.QueryService
	SignService   service.SignService
	ReportService service.ReportService
	// TODO: Add SchoolSearch service if needed, or keep logic in QueryService
}

// NewHandlers creates a new Handlers struct with injected services.
func NewHandlers(authService service.AuthService, queryService service.QueryService, signService service.SignService, reportService service.ReportService, autoSign *autosign.Store, platformAuth *platformauth.Store) *Handlers {
	return &Handlers{
		PlatformAuth:  platformAuth,
		AutoSign:      autoSign,
		AuthService:   authService,
		QueryService:  queryService,
		SignService:   signService,
		ReportService: reportService,
	}
}

// LoginRequest represents the expected login payload
type LoginRequest struct {
	Account    string `json:"account" binding:"required"`
	Password   string `json:"password" binding:"required"`
	SchoolID   string `json:"school_id" binding:"required"`
	SchoolName string `json:"school_name"`
	Token      string `json:"token"`
}

// LoginResponse represents the login response payload
type LoginResponse struct {
	Message string `json:"message"`
	Token   string `json:"token,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleLogin processes the login request - now a method on Handlers
func (h *Handlers) handleLogin(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LoginResponse{
			Message: "请求参数错误",
			Error:   err.Error(),
		})
		return
	}

	if h.PlatformAuth == nil || h.AuthService == nil {
		c.JSON(503, gin.H{"error": "账号连接服务尚未就绪。"})
		return
	}
	if req.Account != strings.TrimSpace(req.Account) || len(req.Account) > 128 || len(req.SchoolName) > 256 {
		c.JSON(400, gin.H{"error": "账号或学校信息无效。"})
		return
	}
	user := currentPlatformUser(c)
	if !h.PlatformAuth.CanBind(user.ID, req.Account) {
		c.JSON(403, gin.H{"error": "该习讯云账号已绑定其他系统用户。"})
		return
	}
	if req.Account != user.StudentNumber {
		c.JSON(400, gin.H{"error": "习讯云学号须与登录学号一致。"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), platformauth.LoginTimeout)
	defer cancel()
	if e := h.PlatformAuth.AllowLogin(req.Account, requestClientIP(c)); e != nil {
		authError(c, e)
		return
	}
	release, e := h.PlatformAuth.LockLogin(ctx, req.Account)
	if e != nil {
		authError(c, e)
		return
	}
	defer release()
	guard := h.PlatformAuth.LoginCommitGuard(req.Account, req.SchoolID)
	if err := h.PlatformAuth.CheckAccountSchool(req.Account, req.SchoolID); err != nil {
		authError(c, err)
		return
	}
	// Verify upstream credentials before recording ownership of legacy data.
	token, err := h.loginWithContext(ctx, req.Account, req.Password, req.SchoolID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Message: "登录失败", // Consider more specific messages if possible
			Error:   err.Error(),
		})
		return
	}

	if err := guard(ctx, func() error { return h.PlatformAuth.Bind(user.ID, req.Account, req.SchoolID, req.SchoolName) }); err != nil {
		authError(c, err)
		return
	}
	_ = token // Upstream credentials stay on the server.
	c.JSON(http.StatusOK, LoginResponse{
		Message: "登录成功",
	})
}

// QueryResponse represents the query response payload
type QueryResponse struct {
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// handleQuery processes the query request - now a method on Handlers
func (h *Handlers) handleQuery(c *gin.Context) {
	account := c.Query("account")
	if account == "" {
		c.JSON(http.StatusBadRequest, QueryResponse{
			Message: "缺少 account 参数",
		})
		return
	}

	month := c.Query("month_date")
	if month != "" {
		if err := service.ValidateSignMonth(month); err != nil {
			c.JSON(http.StatusBadRequest, QueryResponse{Message: "查询月份无效", Error: err.Error()})
			return
		}
	}
	if h.QueryService == nil {
		c.JSON(http.StatusServiceUnavailable, QueryResponse{Message: "签到查询服务尚未就绪，请重启本地服务。"})
		return
	}
	var data map[string]interface{}
	var err error
	if month == "" {
		data, err = h.QueryService.QuerySignInfo(account)
	} else if monthly, ok := h.QueryService.(service.MonthlyQueryService); ok {
		data, err = monthly.QuerySignInfoForMonth(account, month)
	} else {
		c.JSON(http.StatusServiceUnavailable, QueryResponse{Message: "月份查询服务尚未就绪，请重启本地服务。"})
		return
	}
	if err != nil {
		if upstreamSessionFailure(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, QueryResponse{
			Message: "查询失败", // Consider more specific messages
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, QueryResponse{
		Message: "查询成功",
		Data:    data,
	})
}

// SignRequest represents the expected sign-in payload
type SignRequest struct {
	Account           string   `json:"account" binding:"required"`
	Address           string   `json:"address" binding:"required"`
	AddressName       string   `json:"address_name"`
	Latitude          string   `json:"latitude"`
	Longitude         string   `json:"longitude"`
	Remark            string   `json:"remark"`
	Comment           string   `json:"comment"`
	Province          string   `json:"province"`
	City              string   `json:"city"`
	Photos            []string `json:"photos"`
	SecretKey         string   `json:"secret_key"`
	LocationConfirmed bool     `json:"location_confirmed"`
	ConfirmationToken string   `json:"confirmation_token"`
}

// SignResponse represents the sign-in response payload
type SignResponse struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// handleSign processes the sign-in request - now a method on Handlers
func (h *Handlers) handleSign(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req SignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, SignResponse{
			Message: "请求参数错误",
			Error:   err.Error(),
		})
		return
	}
	if _, _, valid := parseLocation(req.Longitude + "," + req.Latitude); !req.LocationConfirmed || !valid {
		c.JSON(http.StatusBadRequest, SignResponse{Message: "请搜索并确认完整地点后再提交签到。"})
		return
	}
	if !verifyLocation(req) {
		c.JSON(http.StatusBadRequest, SignResponse{Message: "地点确认已失效或信息不一致，请重新搜索并确认。"})
		return
	}

	// 调用注入的 SignService
	// Note: The original cmd.SignIn returned a string "签到成功" or an error string.
	// The service.SignIn interface returns (string, error). We adapt the logic.
	if h.SignService == nil {
		c.JSON(503, SignResponse{Message: "签到服务尚未就绪，请重启本地服务。"})
		return
	}
	userID := currentPlatformUser(c).ID
	sessionToken, _ := c.Cookie(authCookie)
	message, err := h.SignService.SignInWithOptions(c.Request.Context(), req.Account, req.Address, req.AddressName, req.Latitude, req.Longitude, service.SignOptions{Remark: req.Remark, Comment: req.Comment, Photos: req.Photos, BeforeSubmit: func() error {
		if h.PlatformAuth == nil {
			return service.ErrSignCancelled
		}
		current, err := h.PlatformAuth.Self(sessionToken)
		if err != nil || current.ID != userID {
			return service.ErrSignCancelled
		}
		return nil
	}})
	if err != nil {
		if errors.Is(err, service.ErrSignCancelled) {
			c.JSON(http.StatusForbidden, SignResponse{Message: "账号状态已变化，签到已取消，请重新登录核对。"})
			return
		}
		if upstreamSessionFailure(c, err) {
			return
		}
		status := http.StatusBadGateway
		if errors.Is(err, service.ErrSignInput) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, service.ErrSignAccount) {
			status = http.StatusUnauthorized
		}
		c.JSON(status, SignResponse{
			Message: "签到失败",
			Error:   err.Error(), // Use the error returned by the service
		})
		return
	}

	// Assuming the service returns a success message string when err is nil
	c.JSON(http.StatusOK, SignResponse{
		Message: message, // Use the message from the service
	})
}

// SearchSchoolResponse represents the search school ID response payload
// Note: utils.SchoolInfo is used here. Consider moving SchoolInfo definition
// to the service package if it's primarily a service-level concept,
// or keep it in utils if it's a general utility struct.
type SearchSchoolResponse struct {
	Message string               `json:"message"`
	Schools []service.SchoolInfo `json:"schools,omitempty"` // Use service.SchoolInfo
	Error   string               `json:"error,omitempty"`
}

// handleSearchSchoolID processes the search school ID request - now a method on Handlers
func (h *Handlers) handleSearchSchoolID(c *gin.Context) {
	// 获取查询参数
	schoolName := c.Query("school_name")
	if schoolName == "" {
		c.JSON(http.StatusBadRequest, SearchSchoolResponse{
			Message: "缺少 school_name 参数",
		})
		return
	}

	// 调用注入的 QueryService's SearchSchool method
	// Assuming QueryService handles the school search logic now.
	// The original cmd.SearchSchoolID returned []utils.SchoolInfo, error.
	// The service.SearchSchool interface returns ([]service.SchoolInfo, error).
	// We need to ensure the types match or adapt. Let's assume service.SchoolInfo exists
	// and is compatible or identical to utils.SchoolInfo for now.
	// If not, we'd need to adjust the service interface or the handler response.
	schools, err := h.QueryService.SearchSchool(schoolName) // Using QueryService
	if err != nil {
		c.JSON(http.StatusInternalServerError, SearchSchoolResponse{
			Message: "查询失败", // Consider more specific messages
			Error:   err.Error(),
		})
		return
	}

	// 如果没有找到匹配的学校
	if len(schools) == 0 {
		c.JSON(http.StatusNotFound, SearchSchoolResponse{
			Message: "没有找到匹配的学校",
		})
		return
	}

	// 返回查询成功结果
	// Adapt the response to use the type returned by the service.
	// Assuming service.SchoolInfo is compatible with utils.SchoolInfo for JSON marshalling.
	// If service.SchoolInfo is different, we might need a conversion step here.
	c.JSON(http.StatusOK, SearchSchoolResponse{
		Message: "查询成功",
		Schools: schools, // Use the result from QueryService.SearchSchool
	})
}
