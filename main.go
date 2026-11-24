package main

import (
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func init() {
	// Alpine 等精简系统可能缺少 mailcap/mime.types，手动注册常用类型作为兜底
	mimes := map[string]string{
		".json": "application/json; charset=utf-8",
		".txt":  "text/plain; charset=utf-8",
		".md":   "text/markdown; charset=utf-8",
		".html": "text/html; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".js":   "application/javascript; charset=utf-8",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".pdf":  "application/pdf",
		".zip":  "application/zip",
		".bin":  "application/octet-stream",
	}
	for ext, typ := range mimes {
		_ = mime.AddExtensionType(ext, typ)
	}
}

func parseBoolEnv(raw string, defaultVal bool) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultVal, nil
	}
	return strconv.ParseBool(trimmed)
}

// newMux 装配 HTTP 路由：GET/HEAD 读文件（目录列表按 PUBLIC_DIR_LIST 控制）、PUT/POST/DELETE 鉴权
func newMux(h *handler) *http.ServeMux {
	mux := http.NewServeMux()
	// GET 路由同时匹配 HEAD。
	mux.HandleFunc("GET /{path...}", h.handleGet)
	mux.HandleFunc("PUT /{path...}", requireAuth(h.token, h.handlePut))
	mux.HandleFunc("POST /{path...}", requireAuth(h.token, h.handlePost))
	mux.HandleFunc("DELETE /{path...}", requireAuth(h.token, h.handleDelete))
	return mux
}

func main() {
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	publicDirList, err := parseBoolEnv(os.Getenv("PUBLIC_DIR_LIST"), false)
	if err != nil {
		log.Fatalf("invalid PUBLIC_DIR_LIST value %q: %v", os.Getenv("PUBLIC_DIR_LIST"), err)
	}

	// 先去除末尾斜杠，再转绝对路径；Abs 内部会 Clean，TrimRight 是 Abs 失败时的兜底
	dataDir = strings.TrimRight(dataDir, "/")
	if absDir, err := filepath.Abs(dataDir); err == nil {
		dataDir = absDir
	}

	h := &handler{dataDir: dataDir, token: token, publicDirList: publicDirList}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           withCORS(newMux(h)),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// 故意不设 ReadTimeout / WriteTimeout，以支持大文件的慢上传/慢下载
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("restfs listening on :%s, data dir: %s, public dir list: %t", port, dataDir, publicDirList)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Printf("received %s, shutting down...", sig)
	case err := <-errCh:
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
