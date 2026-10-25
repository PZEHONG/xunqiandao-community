package web

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
	"xixunyunsign/service"
)

func reportFailure(c *gin.Context, err error) {
	if upstreamSessionFailure(c, err) {
		return
	}
	status := http.StatusBadGateway
	if errors.Is(err, service.ErrReportInput) {
		status = http.StatusBadRequest
	}
	if errors.Is(err, service.ErrReportAccount) {
		status = http.StatusUnauthorized
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
func (h *Handlers) reportReady(c *gin.Context, account string) bool {
	if strings.TrimSpace(account) == "" {
		c.JSON(400, gin.H{"error": "请先填写并连接习讯云账号。"})
		return false
	}
	if h.ReportService == nil {
		c.JSON(503, gin.H{"error": "报告服务尚未就绪，请重启本地服务。"})
		return false
	}
	return true
}
func (h *Handlers) handleReportList(c *gin.Context) {
	account := c.Query("account")
	if !h.reportReady(c, account) {
		return
	}
	page, size := 1, 15
	for _, param := range []struct {
		name   string
		target *int
	}{{"page", &page}, {"page_size", &size}} {
		if value, exists := c.GetQuery(param.name); exists {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || (param.name == "page_size" && n > 100) {
				c.JSON(400, gin.H{"error": "分页参数无效。"})
				return
			}
			*param.target = n
		}
	}
	result, err := h.ReportService.List(c.Request.Context(), account, service.ReportFilter{BusinessType: c.Query("business_type"), Page: page, PageSize: size, StartTime: c.Query("start_time"), EndTime: c.Query("end_time"), MarkState: c.Query("mark_state")})
	if err != nil {
		reportFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"data": result})
}
func (h *Handlers) handleReportTemplate(c *gin.Context) {
	account := c.Query("account")
	if !h.reportReady(c, account) {
		return
	}
	fields, err := h.ReportService.Template(c.Request.Context(), account, c.Query("business_type"))
	if err != nil {
		reportFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"data": fields})
}
func (h *Handlers) handleReportDetail(c *gin.Context) {
	account := c.Query("account")
	if !h.reportReady(c, account) {
		return
	}
	if strings.TrimSpace(c.Query("id")) == "" {
		c.JSON(400, gin.H{"error": "缺少报告编号。"})
		return
	}
	result, err := h.ReportService.Detail(c.Request.Context(), account, c.Query("id"))
	if err != nil {
		reportFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"data": result})
}
func (h *Handlers) handleReportSubmit(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var input struct {
		Account string `json:"account" binding:"required"`
		service.ReportSubmission
	}
	if c.ShouldBindJSON(&input) != nil || input.BusinessType == "" || input.StartDate == "" || input.EndDate == "" || len(input.Content) == 0 {
		c.JSON(400, gin.H{"error": "请完整填写报告日期与内容。"})
		return
	}
	if !h.reportReady(c, input.Account) {
		return
	}
	result, err := h.ReportService.Submit(c.Request.Context(), input.Account, input.ReportSubmission)
	if err != nil {
		reportFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"message": "报告提交成功。", "data": result})
}
func (h *Handlers) handleReportUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxReportFileSize+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		c.JSON(400, gin.H{"error": "无法读取附件，单个文件最多 10 MB。"})
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	account := c.PostForm("account")
	if !h.reportReady(c, account) {
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "请选择附件。"})
		return
	}
	defer file.Close()
	if header.Size == 0 || header.Size > service.MaxReportFileSize {
		c.JSON(400, gin.H{"error": "单个附件需为 1 字节至 10 MB。"})
		return
	}
	uri, err := h.ReportService.Upload(c.Request.Context(), account, header.Filename, file)
	if err != nil {
		reportFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"uri": uri})
}
