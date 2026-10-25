package web

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"xixunyunsign/service"
)

func signFailure(c *gin.Context, err error) {
	if upstreamSessionFailure(c, err) {
		return
	}
	status := http.StatusBadGateway
	if errors.Is(err, service.ErrSignInput) {
		status = 400
	}
	if errors.Is(err, service.ErrSignAccount) {
		status = 401
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func (h *Handlers) signReady(c *gin.Context, account string) bool {
	if strings.TrimSpace(account) == "" {
		c.JSON(400, gin.H{"error": "请先连接习讯云账号。"})
		return false
	}
	if h.SignService == nil {
		c.JSON(503, gin.H{"error": "签到服务尚未就绪，请重启本地服务。"})
		return false
	}
	return true
}

func (h *Handlers) handleSignRemarks(c *gin.Context) {
	account := c.Query("account")
	if !h.signReady(c, account) {
		return
	}
	remarks, err := h.SignService.Remarks(c.Request.Context(), account)
	if err != nil {
		signFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"remarks": remarks})
}

func (h *Handlers) handleSignPhoto(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxSignPhotoSize+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		c.JSON(400, gin.H{"error": "无法读取照片，单张最多 10 MB。"})
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	account := c.PostForm("account")
	if !h.signReady(c, account) {
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "请选择照片。"})
		return
	}
	defer file.Close()
	if header.Size == 0 || header.Size > service.MaxSignPhotoSize {
		c.JSON(400, gin.H{"error": "单张照片需为 1 字节至 10 MB。"})
		return
	}
	uri, err := h.SignService.UploadPhoto(c.Request.Context(), account, header.Filename, file)
	if err != nil {
		signFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"uri": uri})
}
