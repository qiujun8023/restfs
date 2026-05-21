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

	// 先去除末尾斜杠，再转绝对路径；Abs 内部会 Clean，TrimRight 是 Abs 失败时的兜底
	dataDir = strings.TrimRight(dataDir, "/")
	if absDir, err := filepath.Abs(dataDir); err == nil {
		dataDir = absDir
	}

	h := &handler{dataDir: dataDir, token: token}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{path...}", h.handleGet)
	mux.HandleFunc("HEAD /{path...}", h.handleGet)
	mux.HandleFunc("PUT /{path...}", requireAuth(token, h.handlePut))
	mux.HandleFunc("POST /{path...}", requireAuth(token, h.handlePost))
	mux.HandleFunc("DELETE /{path...}", requireAuth(token, h.handleDelete))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// 故意不设 ReadTimeout / WriteTimeout，以支持大文件的慢上传/慢下载
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("restfs listening on :%s, data dir: %s", port, dataDir)
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
