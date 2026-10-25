package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:static
var uiFiles embed.FS

func registerUI(router *gin.Engine) {
	registerUIFiles(router, uiFiles)
}

func registerUIFiles(router *gin.Engine, files fs.FS) {
	assets, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	legacy := func(c *gin.Context) {
		page, err := fs.ReadFile(files, "static/index.html")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", page)
	}
	router.GET("/legacy/", legacy)
	router.GET("/", func(c *gin.Context) {
		page, err := fs.ReadFile(files, "static/berry/index.html")
		if err != nil {
			legacy(c)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", page)
	})
	router.StaticFS("/assets", http.FS(assets))
	registerBerryUI(router, files)
	for route, file := range map[string]string{"/terms": "static/terms.html", "/privacy": "static/privacy.html"} {
		file := file
		router.GET(route, func(c *gin.Context) {
			page, err := fs.ReadFile(files, file)
			if err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", page)
		})
	}
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}

// Only the known application entry serves HTML. Missing chunks remain 404s.
// The explicit legacy entry remains available independently of Berry assets.
func registerBerryUI(router *gin.Engine, files fs.FS) {
	router.GET("/next/*path", func(c *gin.Context) {
		path := c.Param("path")
		if path == "/" {
			page, err := fs.ReadFile(files, "static/berry/index.html")
			if err != nil {
				c.Header("Cache-Control", "no-store")
				c.String(http.StatusServiceUnavailable, "新版前端尚未构建，请访问 /legacy/ 使用旧版。")
				return
			}
			c.Header("Cache-Control", "no-store")
			c.Data(http.StatusOK, "text/html; charset=utf-8", page)
			return
		}
		name := strings.TrimPrefix(path, "/")
		if !fs.ValidPath(name) {
			c.Status(http.StatusNotFound)
			return
		}
		assets, err := fs.Sub(files, "static/berry")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		info, err := fs.Stat(assets, name)
		if err != nil || !info.Mode().IsRegular() {
			c.Status(http.StatusNotFound)
			return
		}
		c.FileFromFS(name, http.FS(assets))
	})
}
