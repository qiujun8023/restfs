package main

import (
	"log"
	"net/http"
	"os"
	"strings"
)

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

	// 确保 dataDir 以 / 结尾
	dataDir = strings.TrimRight(dataDir, "/")

	h := &handler{dataDir: dataDir, token: token}

	mux := http.NewServeMux()

	// GET: 读文件 / 列目录（无需鉴权），/{path...} 本身也匹配根路径
	mux.HandleFunc("GET /{path...}", h.handleGet)

	// PUT: 上传文件（需鉴权）
	mux.HandleFunc("PUT /{path...}", requireAuth(token, h.handlePut))

	// POST: 表单上传（需鉴权）
	mux.HandleFunc("POST /{path...}", requireAuth(token, h.handlePost))

	// DELETE: 删除文件（需鉴权，不支持删目录）
	mux.HandleFunc("DELETE /{path...}", requireAuth(token, h.handleDelete))

	log.Printf("depot listening on :%s, data dir: %s", port, dataDir)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
