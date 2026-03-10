package panel

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ============================================================
// 前端面板嵌入 — 通过 go:embed 提供 SPA 静态资源
// 构建前端后，dist 目录嵌入到二进制中
// ============================================================

//go:embed web/dist/*
var distFS embed.FS

// RegisterRoutes 注册前端面板路由
// 采用 SPA 回退策略：非静态资源请求返回 index.html
func RegisterRoutes(engine *gin.Engine, basePath string) {
	if basePath == "" {
		basePath = "/panel"
	}
	basePath = "/" + strings.Trim(strings.TrimSpace(basePath), "/")

	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		// dist 目录不存在（前端未构建），静默跳过
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	serveIndex := func(c *gin.Context) {
		c.FileFromFS("/index.html", http.FS(sub))
	}

	engine.GET(basePath, func(c *gin.Context) {
		c.Redirect(http.StatusFound, basePath+"/")
	})

	panelGroup := engine.Group(basePath)
	panelGroup.GET("/", serveIndex)
	panelGroup.GET("/:filepath", func(c *gin.Context) {
		path := c.Param("filepath")
		if path == "" {
			serveIndex(c)
			return
		}

		// SPA 回退: 非静态资源请求返回 index.html
		f, err := sub.Open(path)
		if err != nil {
			serveIndex(c)
			return
		}
		f.Close()

		c.Request.URL.Path = "/" + path
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	panelGroup.GET("/:dir/*filepath", func(c *gin.Context) {
		dir := c.Param("dir")
		filepath := strings.TrimPrefix(c.Param("filepath"), "/")
		if dir == "" {
			serveIndex(c)
			return
		}

		path := dir
		if filepath != "" {
			path += "/" + filepath
		}

		// SPA 回退: 非静态资源请求返回 index.html
		f, err := sub.Open(path)
		if err != nil {
			serveIndex(c)
			return
		}
		f.Close()

		c.Request.URL.Path = "/" + path
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
