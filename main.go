package main

import (
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
)

func init() {
	// Alpine 等精简系统可能缺少 mailcap/mime.types，手动注册常用类型作为兜底
	mimes := map[string]string{
		".json": "application/json; charset=utf-8",
		".txt":  "text/plain; charset=utf-8",
		".md":   "text/markdown; charset=utf-8",
		".html": "text/html; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".js":   "application/javascript",
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

	// 去掉末尾的 /，保证拼接路径时格式统一
	dataDir = strings.TrimRight(dataDir, "/")

	h := &handler{dataDir: dataDir, token: token}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{path...}", h.handleGet)
	mux.HandleFunc("PUT /{path...}", requireAuth(token, h.handlePut))
	mux.HandleFunc("POST /{path...}", requireAuth(token, h.handlePost))
	mux.HandleFunc("DELETE /{path...}", requireAuth(token, h.handleDelete))

	log.Printf("restfs listening on :%s, data dir: %s", port, dataDir)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
