package panel

import (
	"embed"
	"io"
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

	// 读取 index.html 内容用于 SPA 回退
	indexPath := "index.html"
	indexContent, err := fs.ReadFile(sub, indexPath)
	if err != nil {
		// index.html 不存在，静默跳过
		return
	}

	serveIndex := func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexContent)
	}

	serveStatic := func(c *gin.Context, path string) {
		// 打开静态文件
		f, err := sub.Open(path)
		if err != nil {
			serveIndex(c)
			return
		}
		defer f.Close()

		// 获取文件信息以确定 Content-Type
		stat, err := f.Stat()
		if err != nil {
			serveIndex(c)
			return
		}

		// 如果是目录，返回 index.html
		if stat.IsDir() {
			serveIndex(c)
			return
		}

		// 设置 Content-Type
		ext := path[strings.LastIndex(path, "."):]
		contentType := "application/octet-stream"
		switch ext {
		case ".js":
			contentType = "application/javascript"
		case ".css":
			contentType = "text/css"
		case ".html":
			contentType = "text/html; charset=utf-8"
		case ".json":
			contentType = "application/json"
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		case ".svg":
			contentType = "image/svg+xml"
		case ".ico":
			contentType = "image/x-icon"
		case ".woff", ".woff2":
			contentType = "font/woff2"
		case ".ttf":
			contentType = "font/ttf"
		}

		c.DataFromReader(http.StatusOK, stat.Size(), contentType, f, nil)
	}

	// 首页路由
	engine.GET(basePath, func(c *gin.Context) {
		c.Redirect(http.StatusFound, basePath+"/")
	})

	// 面板根路径
	engine.GET(basePath+"/", serveIndex)

	// 静态资源路由 - assets 目录
	engine.GET(basePath+"/assets/*filepath", func(c *gin.Context) {
		filepath := strings.TrimPrefix(c.Param("filepath"), "/")
		serveStatic(c, "assets/"+filepath)
	})

	// 常见静态文件
	engine.GET(basePath+"/favicon.ico", func(c *gin.Context) {
		serveStatic(c, "favicon.ico")
	})
	engine.GET(basePath+"/vite.svg", func(c *gin.Context) {
		serveStatic(c, "vite.svg")
	})

	// SPA 回退：所有未匹配的 /panel/* 路径返回 index.html
	engine.NoRoute(func(c *gin.Context) {
		// 只处理 GET 请求
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		// 检查是否是面板路径
		path := c.Request.URL.Path
		if path == basePath || strings.HasPrefix(path, basePath+"/") {
			serveIndex(c)
			c.Abort()
			return
		}

		c.Next()
	})
}

// init 在包初始化时预读取 index.html
func init() {
	// 预热：确保 distFS 可用
	sub, err := fs.Sub(distFS, "web/dist")
	if err == nil {
		// 尝试读取 index.html 以便提前发现问题
		f, err := sub.Open("index.html")
		if err == nil {
			// 读取并丢弃内容，仅用于验证文件存在
			io.Copy(io.Discard, f)
			f.Close()
		}
	}
}
