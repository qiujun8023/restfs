package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type handler struct {
	dataDir string
	token   string
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// resolvePath 将 URL 路径解析为绝对文件系统路径，防止路径穿越
func (h *handler) resolvePath(urlPath string) (string, bool) {
	// 清理路径
	clean := filepath.Join(h.dataDir, filepath.FromSlash("/"+urlPath))
	// 确保在 dataDir 内
	if !strings.HasPrefix(clean, h.dataDir+string(filepath.Separator)) && clean != h.dataDir {
		return "", false
	}
	return clean, true
}

// ---------- GET ----------

func (h *handler) handleGet(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")

	fsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	if info.IsDir() {
		h.serveDir(w, r, fsPath, urlPath)
		return
	}

	// 文件：直接返回内容，http.ServeFile 自动处理 MIME、Range、缓存等
	http.ServeFile(w, r, fsPath)
}

// dirEntry 是文件/目录的统一元数据结构
type dirEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "file" | "directory"
	Size     int64  `json:"size,omitempty"`
	URL      string `json:"url"`
	Modified string `json:"modified"`
}

func buildDirEntry(info os.FileInfo, basePath string) dirEntry {
	name := info.Name()
	modTime := info.ModTime().UTC().Format(time.RFC3339)
	de := dirEntry{
		Name:     name,
		Modified: modTime,
	}

	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	if info.IsDir() {
		de.Type = "directory"
		de.Name += "/"
		de.URL = basePath + url.PathEscape(name) + "/"
	} else {
		de.Type = "file"
		de.Size = info.Size()
		de.URL = basePath + url.PathEscape(name)
	}
	return de
}

func (h *handler) serveDir(w http.ResponseWriter, r *http.Request, fsPath, urlPath string) {
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	displayPath := "/" + strings.TrimLeft(urlPath, "/")
	if displayPath != "/" && !strings.HasSuffix(displayPath, "/") {
		displayPath += "/"
	}

	// 构建条目列表：文件夹优先，按名称排序
	var dirs, files []dirEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		de := buildDirEntry(info, displayPath)
		if e.IsDir() {
			dirs = append(dirs, de)
		} else {
			files = append(files, de)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	all := append(dirs, files...)
	if all == nil {
		all = []dirEntry{}
	}

	// Content Negotiation
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		writeJSON(w, http.StatusOK, all)
		return
	}

	// 默认 HTML
	// 读取 README.md 内容（如果有）
	var readmeHTML string
	readmePath := filepath.Join(fsPath, "README.md")
	if data, err := os.ReadFile(readmePath); err == nil {
		readmeHTML = renderMarkdown(data)
	}

	renderDirHTML(w, displayPath, all, readmeHTML)
}

// ---------- PUT ----------

func (h *handler) handlePut(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")
	if urlPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}

	fsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	// 不允许写到目录
	if info, err := os.Stat(fsPath); err == nil && info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is a directory"})
		return
	}

	isNew := true
	if _, err := os.Stat(fsPath); err == nil {
		isNew = false
	}

	if err := os.MkdirAll(filepath.Dir(fsPath), 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	f, err := os.Create(fsPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, r.Body); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("PUT %s", fsPath)
	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	if info, err := os.Stat(fsPath); err == nil {
		basePath := filepath.Dir("/" + strings.TrimLeft(urlPath, "/"))
		writeJSON(w, status, buildDirEntry(info, basePath))
	} else {
		writeJSON(w, status, map[string]string{"path": "/" + strings.TrimLeft(urlPath, "/")})
	}
}

// ---------- POST (multipart) ----------

func (h *handler) handlePost(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")

	dirFsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	// 32MB 内存，超出写临时文件
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "field 'file' required"})
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	if filename == "" || filename == "." {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}

	fsPath := filepath.Join(dirFsPath, filename)
	// 再次校验最终路径
	if !strings.HasPrefix(fsPath, h.dataDir+string(filepath.Separator)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	isNew := true
	if _, err := os.Stat(fsPath); err == nil {
		isNew = false
	}

	if err := os.MkdirAll(dirFsPath, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	dst, err := os.Create(fsPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("POST %s", fsPath)
	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}

	if info, err := os.Stat(fsPath); err == nil {
		basePath := "/" + strings.TrimLeft(urlPath, "/")
		writeJSON(w, status, buildDirEntry(info, basePath))
	} else {
		// 降级处理
		resultPath := "/" + strings.TrimLeft(strings.TrimPrefix(fsPath, h.dataDir), "/")
		writeJSON(w, status, map[string]string{"path": resultPath})
	}
}

// ---------- DELETE ----------

func (h *handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")
	if urlPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}

	fsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	if info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete directory"})
		return
	}

	if err := os.Remove(fsPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("DELETE %s", fsPath)

	// 同步自底向上清理所有空父目录
	pruneEmptyDirs(filepath.Dir(fsPath), h.dataDir)

	w.WriteHeader(http.StatusNoContent)
}

// pruneEmptyDirs 自底向上递归删除空目录，直到 root 为止（root 本身不删）
func pruneEmptyDirs(dir, root string) {
	for {
		if dir == root || !strings.HasPrefix(dir, root) {
			break
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil {
			break
		}
		log.Printf("pruned empty dir: %s", dir)
		dir = filepath.Dir(dir)
	}
}
