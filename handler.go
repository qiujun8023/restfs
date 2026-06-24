package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type handler struct {
	dataDir string
	token   string
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// resolvePath 将 URL 路径解析为绝对文件系统路径，防止路径穿越
func (h *handler) resolvePath(urlPath string) (string, bool) {
	clean := filepath.Join(h.dataDir, filepath.FromSlash("/"+urlPath))
	if clean == h.dataDir {
		return clean, true
	}
	if !strings.HasPrefix(clean, h.dataDir+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

// escapeURLPath 对路径每段单独 URL 转义，返回以 "/" 开头、不带尾斜杠的路径
func escapeURLPath(p string) string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return "/"
	}
	segs := strings.Split(trimmed, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return "/" + strings.Join(segs, "/")
}

// urlDirOf 返回 URL 路径的父目录（已 URL 转义，带尾斜杠）
func urlDirOf(urlPath string) string {
	cleaned := path.Clean("/" + strings.TrimLeft(urlPath, "/"))
	parent := path.Dir(cleaned)
	if parent == "/" {
		return "/"
	}
	return escapeURLPath(parent) + "/"
}

// urlAsDir 将 URL 路径本身视为目录返回（已 URL 转义，带尾斜杠）
func urlAsDir(urlPath string) string {
	cleaned := path.Clean("/" + strings.TrimLeft(urlPath, "/"))
	if cleaned == "/" {
		return "/"
	}
	return escapeURLPath(cleaned) + "/"
}

func (h *handler) handleGet(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")

	fsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	if info.IsDir() && !wantsJSON(r) && urlPath != "" && !strings.HasSuffix(r.URL.Path, "/") {
		target := urlAsDir(urlPath)
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}

	log.Printf("%s /%s", r.Method, urlPath)
	if info.IsDir() {
		h.serveDir(w, r, fsPath, urlPath)
		return
	}
	http.ServeFile(w, r, fsPath)
}

type dirEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "file" | "directory"
	Size     int64  `json:"size,omitempty"`
	Path     string `json:"path"`
	Modified string `json:"modified"`
}

// buildDirEntry 构造目录条目；basePath 必须是已 URL 转义、带尾斜杠的目录 URL
func buildDirEntry(info os.FileInfo, basePath string) dirEntry {
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	name := info.Name()
	de := dirEntry{
		Name:     name,
		Modified: info.ModTime().UTC().Format(time.RFC3339),
	}
	if info.IsDir() {
		de.Type = "directory"
		de.Name += "/"
		de.Path = basePath + url.PathEscape(name) + "/"
	} else {
		de.Type = "file"
		de.Size = info.Size()
		de.Path = basePath + url.PathEscape(name)
	}
	return de
}

// readmeScore 给 readme 文件名打分：README.md(3) > readme.md(2) > README.MD(1) > 其他大小写变体(0)。
// 非 readme.md 返回 -1
func readmeScore(name string) int {
	if !strings.EqualFold(name, "readme.md") {
		return -1
	}
	switch name {
	case "README.md":
		return 3
	case "readme.md":
		return 2
	case "README.MD":
		return 1
	default:
		return 0
	}
}

// wantsJSON 决定目录列表是否以 JSON 返回。
// 优先级：?format=json > 浏览器（Accept 含 text/html）→ HTML > Accept 含 application/json → JSON > HTML
func wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		return false
	}
	return strings.Contains(accept, "application/json")
}

func (h *handler) serveDir(w http.ResponseWriter, r *http.Request, fsPath, urlPath string) {
	// 根据 Accept 切换 HTML/JSON：提示中间缓存按 Accept 分流
	w.Header().Set("Vary", "Accept")

	if !wantsJSON(r) {
		indexPath := filepath.Join(fsPath, "index.html")
		if info, err := os.Stat(indexPath); err == nil {
			if !info.IsDir() {
				http.ServeFile(w, r, indexPath)
				return
			}
		} else if !os.IsNotExist(err) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	entries, err := os.ReadDir(fsPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	displayPath := "/" + strings.TrimLeft(urlPath, "/")
	if displayPath != "/" && !strings.HasSuffix(displayPath, "/") {
		displayPath += "/"
	}
	basePath := urlAsDir(urlPath)

	var dirs, files []dirEntry
	var bestReadmeName string
	bestReadmeScore := -1
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		de := buildDirEntry(info, basePath)
		if e.IsDir() {
			dirs = append(dirs, de)
			continue
		}
		files = append(files, de)
		if score := readmeScore(e.Name()); score > bestReadmeScore {
			bestReadmeScore = score
			bestReadmeName = e.Name()
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	all := make([]dirEntry, 0, len(dirs)+len(files))
	all = append(all, dirs...)
	all = append(all, files...)

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, all)
		return
	}

	var readmeHTML string
	if bestReadmeName != "" {
		if data, err := os.ReadFile(filepath.Join(fsPath, bestReadmeName)); err == nil {
			readmeHTML = renderMarkdown(data)
		}
	}
	renderDirHTML(w, displayPath, all, bestReadmeName, readmeHTML)
}

// atomicWrite 将 r 的内容原子写入 dst：先写临时文件再 rename，避免读到不完整内容
func atomicWrite(dst string, r io.Reader) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 成功 rename 后此 Remove 为空操作

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

// writeAndRespond 将 src 原子写入 fsPath，并以条目元信息响应。
// parentURL 是 fsPath 所在目录的 URL（已转义，带尾斜杠），用于构造响应中的 path
func (h *handler) writeAndRespond(w http.ResponseWriter, r *http.Request, fsPath, parentURL string, src io.Reader) {
	info, statErr := os.Stat(fsPath)
	switch {
	case statErr == nil && info.IsDir():
		writeError(w, http.StatusBadRequest, "path is a directory")
		return
	case statErr != nil && !errors.Is(statErr, os.ErrNotExist):
		writeError(w, http.StatusInternalServerError, statErr.Error())
		return
	}
	isNew := statErr != nil

	if err := os.MkdirAll(filepath.Dir(fsPath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := atomicWrite(fsPath, src); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("%s %s", r.Method, r.URL.Path)

	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	if info, err := os.Stat(fsPath); err == nil {
		writeJSON(w, status, buildDirEntry(info, parentURL))
		return
	}
	writeJSON(w, status, map[string]string{
		"path": parentURL + url.PathEscape(filepath.Base(fsPath)),
	})
}

func (h *handler) handlePut(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")
	if urlPath == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}
	fsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	h.writeAndRespond(w, r, fsPath, urlDirOf(urlPath), r.Body)
}

func (h *handler) handlePost(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")

	dirFsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	// 32MB 内存，超出写临时文件
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "field 'file' required")
		return
	}
	defer file.Close()

	// 客户端必须直接提供 basename；任何含路径分隔符的 filename 都拒绝，
	// 不能让 filepath.Base 静默剥掉，否则 "a/b" 会被默默写成 "b"
	filename := header.Filename
	if !isSafeFilename(filename) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	fsPath := filepath.Join(dirFsPath, filename)
	if !strings.HasPrefix(fsPath, h.dataDir+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	h.writeAndRespond(w, r, fsPath, urlAsDir(urlPath), file)
}

// isSafeFilename 拒绝可能逃出目标目录或含非法字符的文件名
func isSafeFilename(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r == 0 {
			return false
		}
	}
	return true
}

func (h *handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")
	if urlPath == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}

	fsPath, ok := h.resolvePath(urlPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "cannot delete directory")
		return
	}

	if err := os.Remove(fsPath); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("DELETE /%s", urlPath)
	pruneEmptyDirs(filepath.Dir(fsPath), h.dataDir)

	w.WriteHeader(http.StatusNoContent)
}

// pruneEmptyDirs 自底向上递归删除空目录，直到 root 为止（root 本身不删）
func pruneEmptyDirs(dir, root string) {
	sep := string(filepath.Separator)
	for dir != root && strings.HasPrefix(dir, root+sep) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		log.Printf("PRUNE %s", dir)
		dir = filepath.Dir(dir)
	}
}
