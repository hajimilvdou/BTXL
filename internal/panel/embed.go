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
	servePath := func(c *gin.Context, path string) {
		path = strings.Trim(strings.TrimSpace(path), "/")
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
	}

	engine.GET(basePath, func(c *gin.Context) {
		c.Redirect(http.StatusFound, basePath+"/")
	})

	engine.GET(basePath+"/", serveIndex)
	engine.GET(basePath+"/assets/*filepath", func(c *gin.Context) {
		servePath(c, "assets/"+c.Param("filepath"))
	})
	engine.GET(basePath+"/favicon.ico", func(c *gin.Context) {
		servePath(c, "favicon.ico")
	})
	engine.GET(basePath+"/vite.svg", func(c *gin.Context) {
		servePath(c, "vite.svg")
	})
	engine.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}
		if c.Request.URL.Path == basePath || strings.HasPrefix(c.Request.URL.Path, basePath+"/") {
			serveIndex(c)
			c.Abort()
			return
		}
		c.Next()
	})
}
