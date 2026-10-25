package web

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"xixunyunsign/autosign"
)

type autoSignRequest struct {
	autosign.Task
	Confirmed         bool   `json:"confirmed"`
	LocationConfirmed bool   `json:"location_confirmed"`
	ConfirmationToken string `json:"confirmation_token"`
}
type autoStateRequest struct {
	Account   string `json:"account"`
	Revision  int64  `json:"revision"`
	Enabled   bool   `json:"enabled"`
	Confirmed bool   `json:"confirmed"`
}

func autoBody(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(v) != nil {
		c.JSON(400, gin.H{"error": "请求信息不完整或格式有误"})
		return false
	}
	var extra any
	if decoder.Decode(&extra) == io.EOF {
		return true
	}
	c.JSON(400, gin.H{"error": "请求只能包含一份任务信息"})
	return false
}
func autoError(c *gin.Context, e error) {
	status := 400
	if errors.Is(e, autosign.ErrNotFound) {
		status = 404
	}
	if errors.Is(e, autosign.ErrConflict) {
		status = 409
	}
	c.JSON(status, gin.H{"error": e.Error()})
}
func autoID(c *gin.Context) (int64, bool) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id < 1 {
		c.JSON(400, gin.H{"error": "任务编号无效"})
		return 0, false
	}
	return id, true
}
func registerAutoSignRoutes(router *gin.Engine, h *Handlers) {
	group := router.Group("/api/auto-sign")
	group.Use(func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != c.Request.Host {
				c.AbortWithStatusJSON(403, gin.H{"error": "请从本机工作台操作自动签到"})
				return
			}
		}
		if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			c.AbortWithStatusJSON(403, gin.H{"error": "请从本机工作台操作自动签到"})
			return
		}
		if h.AutoSign == nil {
			c.AbortWithStatusJSON(503, gin.H{"error": "自动签到服务尚未就绪，请重启后台服务"})
			return
		}
		c.Next()
	})
	group.GET("/tasks", h.autoTasks)
	group.POST("/tasks", h.autoSave)
	group.PUT("/tasks/:id", h.autoSave)
	group.DELETE("/tasks/:id", h.autoDelete)
	group.POST("/tasks/:id/state", h.autoState)
	group.GET("/runs", func(c *gin.Context) {
		account := strings.TrimSpace(c.Query("account"))
		if account == "" {
			c.JSON(400, gin.H{"error": "请先连接账号"})
			return
		}
		runs, e := h.AutoSign.Runs(account)
		if e != nil {
			c.JSON(500, gin.H{"error": "读取执行记录失败"})
			return
		}
		c.JSON(200, gin.H{"runs": runs})
	})
}
func (h *Handlers) autoTasks(c *gin.Context) {
	account := strings.TrimSpace(c.Query("account"))
	if account == "" {
		c.JSON(400, gin.H{"error": "请先连接账号"})
		return
	}
	tasks, e := h.AutoSign.Tasks(account)
	if e != nil {
		c.JSON(500, gin.H{"error": "读取任务失败"})
		return
	}
	runs, e := h.AutoSign.Runs(account)
	if e != nil {
		c.JSON(500, gin.H{"error": "读取执行记录失败"})
		return
	}
	now := time.Now()
	type item struct {
		autosign.Task
		NextAt   string `json:"next_at"`
		Timezone string `json:"timezone"`
	}
	result := []item{}
	for _, t := range tasks {
		for i := range runs {
			if runs[i].TaskID == t.ID {
				r := runs[i]
				t.LastRun = &r
				break
			}
		}
		next := ""
		if t.Enabled {
			n := t.NextRun(now)
			if t.LastRun != nil && t.LastRun.Date == now.In(autosign.Shanghai).Format("2006-01-02") && n.In(autosign.Shanghai).Format("2006-01-02") == t.LastRun.Date {
				n = n.AddDate(0, 0, 1)
			}
			next = n.Format(time.RFC3339)
		}
		result = append(result, item{t, next, "Asia/Shanghai"})
	}
	school, _ := h.AutoSign.School(account)
	c.JSON(200, gin.H{"tasks": result, "school_id": school, "server_time": now.In(autosign.Shanghai).Format(time.RFC3339), "mode": "local"})
}
func (h *Handlers) autoValidate(c *gin.Context, t *autosign.Task) bool {
	if h.SignService == nil {
		c.JSON(503, gin.H{"error": "签到服务不可用"})
		return false
	}
	school, e := h.AutoSign.School(t.Account)
	if e != nil {
		autoError(c, e)
		return false
	}
	if t.SchoolID != "" && t.SchoolID != school {
		c.JSON(400, gin.H{"error": "学校信息已变化，请刷新并重新核对任务"})
		return false
	}
	t.SchoolID = school
	remarks, e := h.SignService.Remarks(c.Request.Context(), t.Account)
	if e != nil {
		c.JSON(400, gin.H{"error": "无法核验签到类型，请先重新连接账号"})
		return false
	}
	for _, r := range remarks {
		if r.Key == t.Remark && r.Value == t.RemarkLabel {
			return true
		}
	}
	c.JSON(400, gin.H{"error": "签到类型已变化，请刷新类型并重新确认任务"})
	return false
}
func (h *Handlers) autoSave(c *gin.Context) {
	var req autoSignRequest
	if !autoBody(c, &req) {
		return
	}
	if !req.Confirmed || !req.LocationConfirmed || !req.Enabled {
		c.JSON(400, gin.H{"error": "请核对完整信息并确认启用每日任务"})
		return
	}
	if c.Request.Method == "PUT" {
		id, ok := autoID(c)
		if !ok {
			return
		}
		if req.ID != id {
			c.JSON(400, gin.H{"error": "任务编号不一致"})
			return
		}
		old, e := h.AutoSign.Task(req.Account, id)
		if e != nil {
			autoError(c, e)
			return
		}
		if old.Revision != req.Revision {
			autoError(c, autosign.ErrConflict)
			return
		}
		unchanged := old.Address == req.Address && old.AddressName == req.AddressName && old.Latitude == req.Latitude && old.Longitude == req.Longitude
		if !unchanged && !verifyLocation(SignRequest{Address: req.Address, AddressName: req.AddressName, Latitude: req.Latitude, Longitude: req.Longitude, ConfirmationToken: req.ConfirmationToken}) {
			c.JSON(400, gin.H{"error": "地点已变更，请重新搜索并确认地点"})
			return
		}
	} else {
		req.ID = 0
		req.Revision = 0
		if !verifyLocation(SignRequest{Address: req.Address, AddressName: req.AddressName, Latitude: req.Latitude, Longitude: req.Longitude, ConfirmationToken: req.ConfirmationToken}) {
			c.JSON(400, gin.H{"error": "地点确认已失效，请重新搜索并确认地点"})
			return
		}
	}
	if !h.autoValidate(c, &req.Task) {
		return
	}
	task, e := h.AutoSign.Save(req.Task, time.Now())
	if e != nil {
		if errors.Is(e, autosign.ErrConflict) {
			autoError(c, e)
		} else {
			c.JSON(400, gin.H{"error": "无法保存任务，请检查名称、时间、地点和说明后重试"})
		}
		return
	}
	c.JSON(200, gin.H{"task": task, "message": "每日任务已确认并启用，创建不会立即签到"})
}
func (h *Handlers) autoState(c *gin.Context) {
	id, ok := autoID(c)
	if !ok {
		return
	}
	var req autoStateRequest
	if !autoBody(c, &req) {
		return
	}
	t, e := h.AutoSign.Task(req.Account, id)
	if e != nil {
		autoError(c, e)
		return
	}
	if req.Enabled {
		if !req.Confirmed {
			c.JSON(400, gin.H{"error": "请核对已保存的信息后确认重新启用"})
			return
		}
		if e = h.AutoSign.CheckSchool(t); e != nil {
			autoError(c, e)
			return
		}
		originalLabel := t.RemarkLabel
		if !h.autoValidate(c, &t) {
			return
		}
		if t.RemarkLabel != originalLabel {
			c.JSON(400, gin.H{"error": "签到类型名称已变化，请编辑并重新确认任务"})
			return
		}
	}
	task, e := h.AutoSign.SetEnabled(req.Account, id, req.Revision, req.Enabled, time.Now())
	if e != nil {
		autoError(c, e)
		return
	}
	c.JSON(200, gin.H{"task": task})
}
func (h *Handlers) autoDelete(c *gin.Context) {
	id, ok := autoID(c)
	if !ok {
		return
	}
	var req autoStateRequest
	if !autoBody(c, &req) {
		return
	}
	if e := h.AutoSign.Delete(req.Account, id, req.Revision); e != nil {
		autoError(c, e)
		return
	}
	c.JSON(200, gin.H{"message": "任务已删除，执行记录仍保留"})
}
