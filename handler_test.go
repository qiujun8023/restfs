package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newServer(t *testing.T) (http.Handler, string) {
	t.Helper()
	dataDir := t.TempDir()
	h := &handler{dataDir: dataDir, token: "secret"}
	return withCORS(newMux(h, "secret")), dataDir
}

func do(t *testing.T, server http.Handler, method, target string, body io.Reader, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if auth {
		req.Header.Set("Authorization", "Bearer secret")
	}
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	return rr
}

func TestPutCreatesFile(t *testing.T) {
	server, dataDir := newServer(t)
	rr := do(t, server, http.MethodPut, "/foo/bar.txt", strings.NewReader("hello"), true)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "foo", "bar.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("file content = %q, want %q", data, "hello")
	}

	var entry dirEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil {
		t.Fatalf("response: %v\nbody: %s", err, rr.Body.String())
	}
	if entry.Path != "/foo/bar.txt" {
		t.Fatalf("entry.Path = %q, want /foo/bar.txt", entry.Path)
	}
	if entry.Type != "file" || entry.Size != 5 {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestPutOverwriteReturns200(t *testing.T) {
	server, _ := newServer(t)
	do(t, server, http.MethodPut, "/a.txt", strings.NewReader("v1"), true)
	rr := do(t, server, http.MethodPut, "/a.txt", strings.NewReader("v2"), true)
	if rr.Code != http.StatusOK {
		t.Fatalf("overwrite status = %d, want 200", rr.Code)
	}
}

func TestPutPathTraversalRejected(t *testing.T) {
	server, _ := newServer(t)
	// Go 的 ServeMux 在路由前先 path.Clean，所以 /../etc/passwd 会被规整。
	// 这里通过编码绕过路由清理来测 resolvePath。
	req := httptest.NewRequest(http.MethodPut, "/foo", strings.NewReader("x"))
	req.SetPathValue("path", "../etc/passwd")
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	// 走通过 ServeMux 时已被 clean，这里我们主要验证 resolvePath 本身。
	if got, ok := (&handler{dataDir: "/data"}).resolvePath("../etc/passwd"); ok {
		t.Fatalf("resolvePath leak: %q, ok=%v", got, ok)
	}
}

func TestPutRequiresAuth(t *testing.T) {
	server, _ := newServer(t)
	req := httptest.NewRequest(http.MethodPut, "/a.txt", strings.NewReader("x"))
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func multipartBody(t *testing.T, filename, content string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(fw, content)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestPostUploadsFile(t *testing.T) {
	server, dataDir := newServer(t)
	body, ct := multipartBody(t, "report.txt", "data")
	req := httptest.NewRequest(http.MethodPost, "/uploads", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "uploads", "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "data" {
		t.Fatalf("content = %q", data)
	}
	var entry dirEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Path != "/uploads/report.txt" {
		t.Fatalf("entry.Path = %q", entry.Path)
	}
}

func TestPostRejectsUnsafeFilenames(t *testing.T) {
	server, _ := newServer(t)
	// 注意：mime/multipart 已对 filename 跑过 filepath.Base，所以服务端永远看不到
	// 含 "/" 的 filename（在 Linux）。这里测的是反斜杠、点号等仍可绕过 Base 的情形。
	cases := []string{"..", ".", "", "a\\b"}
	for _, name := range cases {
		body, ct := multipartBody(t, name, "x")
		req := httptest.NewRequest(http.MethodPost, "/u", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer secret")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("filename=%q status = %d, want 400; body=%s", name, rr.Code, rr.Body.String())
		}
	}
}

func TestDeletePrunesEmptyDirs(t *testing.T) {
	server, dataDir := newServer(t)
	do(t, server, http.MethodPut, "/a/b/c/x.txt", strings.NewReader("x"), true)
	do(t, server, http.MethodPut, "/a/b/c/y.txt", strings.NewReader("y"), true)

	rr := do(t, server, http.MethodDelete, "/a/b/c/x.txt", nil, true)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rr.Code)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "a", "b", "c")); err != nil {
		t.Fatalf("dir should still exist (y.txt remains): %v", err)
	}

	rr = do(t, server, http.MethodDelete, "/a/b/c/y.txt", nil, true)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rr.Code)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "a")); !os.IsNotExist(err) {
		t.Fatalf("empty parents should be pruned: err=%v", err)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("root dataDir must remain: %v", err)
	}
}

func TestDeleteDirectoryRejected(t *testing.T) {
	server, dataDir := newServer(t)
	os.Mkdir(filepath.Join(dataDir, "dir"), 0o755)
	rr := do(t, server, http.MethodDelete, "/dir", nil, true)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestDeleteNotFound(t *testing.T) {
	server, _ := newServer(t)
	rr := do(t, server, http.MethodDelete, "/nope", nil, true)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestGetDirectoryHTMLByDefault(t *testing.T) {
	server, dataDir := newServer(t)
	os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("a"), 0o644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html,application/json;q=0.9")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "a.txt") {
		t.Fatalf("body missing a.txt: %s", rr.Body.String())
	}
}

func TestGetDirectoryIndexHTMLByDefault(t *testing.T) {
	server, dataDir := newServer(t)
	os.WriteFile(filepath.Join(dataDir, "index.html"), []byte("<h1>Home</h1>"), 0o644)
	os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("a"), 0o644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<h1>Home</h1>") {
		t.Fatalf("body missing index.html content: %s", body)
	}
	if strings.Contains(body, "a.txt") {
		t.Fatalf("directory listing rendered instead of index.html: %s", body)
	}
}

func TestGetDirectoryIndexHTMLDoesNotOverrideJSON(t *testing.T) {
	server, dataDir := newServer(t)
	os.WriteFile(filepath.Join(dataDir, "index.html"), []byte("<h1>Home</h1>"), 0o644)
	os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("a"), 0o644)

	rr := do(t, server, http.MethodGet, "/?format=json", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	var entries []dirEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2: %+v", len(entries), entries)
	}
	names := []string{entries[0].Name, entries[1].Name}
	if !slices.Contains(names, "a.txt") || !slices.Contains(names, "index.html") {
		t.Fatalf("entries missing files: %+v", entries)
	}
}

func TestGetSubdirectoryIndexHTML(t *testing.T) {
	server, dataDir := newServer(t)
	siteDir := filepath.Join(dataDir, "site")
	os.Mkdir(siteDir, 0o755)
	os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("<h1>Site</h1>"), 0o644)
	os.WriteFile(filepath.Join(siteDir, "page.txt"), []byte("page"), 0o644)

	req := httptest.NewRequest(http.MethodGet, "/site/", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<h1>Site</h1>") {
		t.Fatalf("body missing site index.html content: %s", body)
	}
	if strings.Contains(body, "page.txt") {
		t.Fatalf("directory listing rendered instead of site index.html: %s", body)
	}
}

func TestGetDirectoryHTMLRedirectsToSlash(t *testing.T) {
	server, dataDir := newServer(t)
	os.Mkdir(filepath.Join(dataDir, "site"), 0o755)

	req := httptest.NewRequest(http.MethodGet, "/site", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/site/" {
		t.Fatalf("Location = %q, want /site/", loc)
	}
}

func TestGetDirectoryRedirectPreservesQuery(t *testing.T) {
	server, dataDir := newServer(t)
	os.Mkdir(filepath.Join(dataDir, "site"), 0o755)

	req := httptest.NewRequest(http.MethodGet, "/site?x=1&y=%E4%B8%AD", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/site/?x=1&y=%E4%B8%AD" {
		t.Fatalf("Location = %q, want /site/?x=1&y=%%E4%%B8%%AD", loc)
	}
}

func TestGetDirectoryNoSlashWithJSONFormatNotRedirected(t *testing.T) {
	server, dataDir := newServer(t)
	siteDir := filepath.Join(dataDir, "site")
	os.Mkdir(siteDir, 0o755)
	os.WriteFile(filepath.Join(siteDir, "a.txt"), []byte("a"), 0o644)

	// 无尾斜杠 + format=json：应直接返回 JSON，不重定向
	rr := do(t, server, http.MethodGet, "/site?format=json", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no redirect for JSON)", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestGetDirectoryIndexHTMLFallsBackWhenDir(t *testing.T) {
	server, dataDir := newServer(t)
	// index.html 本身是目录：应回退到目录列表，不应 ServeFile
	os.Mkdir(filepath.Join(dataDir, "index.html"), 0o755)
	os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("a"), 0o644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "a.txt") {
		t.Fatalf("expected directory listing fallback, body: %s", body)
	}
}

func TestGetDirectoryJSONViaQuery(t *testing.T) {
	server, dataDir := newServer(t)
	os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("a"), 0o644)
	os.Mkdir(filepath.Join(dataDir, "sub"), 0o755)

	rr := do(t, server, http.MethodGet, "/?format=json", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	var entries []dirEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	// 目录排在文件前
	if entries[0].Type != "directory" || entries[1].Type != "file" {
		t.Fatalf("ordering wrong: %+v", entries)
	}
}

func TestGetDirectoryJSONViaAccept(t *testing.T) {
	server, _ := newServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}

func TestGetFileServesContent(t *testing.T) {
	server, dataDir := newServer(t)
	os.WriteFile(filepath.Join(dataDir, "hello.txt"), []byte("world"), 0o644)
	rr := do(t, server, http.MethodGet, "/hello.txt", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Body.String() != "world" {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestGetUnicodePath(t *testing.T) {
	server, dataDir := newServer(t)
	dir := filepath.Join(dataDir, "我的文件夹")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "笔记.txt"), []byte("hi"), 0o644)

	// 子文件链接应包含 URL 转义后的中间段
	rr := do(t, server, http.MethodGet, "/%E6%88%91%E7%9A%84%E6%96%87%E4%BB%B6%E5%A4%B9/?format=json", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var entries []dirEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d", len(entries))
	}
	// 中间目录段也应转义
	if !strings.Contains(entries[0].Path, "%E6") {
		t.Fatalf("entry.Path not escaped: %q", entries[0].Path)
	}
}

func TestReadmeXSSEscaped(t *testing.T) {
	server, dataDir := newServer(t)
	os.WriteFile(filepath.Join(dataDir, "README.md"), []byte("# T\n<script>alert(1)</script>\n"), 0o644)
	rr := do(t, server, http.MethodGet, "/", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "<script>") {
		t.Fatalf("script tag leaked into HTML: %s", body)
	}
}

func TestResolvePath(t *testing.T) {
	h := &handler{dataDir: "/data"}
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "/data", true},
		{"/", "/data", true},
		{"foo/bar", "/data/foo/bar", true},
		{"/foo/bar", "/data/foo/bar", true},
		{"../etc/passwd", "", false},
		{"foo/../../etc", "", false},
	}
	for _, c := range cases {
		got, ok := h.resolvePath(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("resolvePath(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPruneEmptyDirsBoundary(t *testing.T) {
	root := t.TempDir()
	// 创建 root2 邻居目录（前缀字符串相同但不是子目录），不应被删
	root2 := root + "2"
	if err := os.Mkdir(root2, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(root2)

	// 给 root 内部一个深空链
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0o755)

	pruneEmptyDirs(deep, root)

	if _, err := os.Stat(root); err != nil {
		t.Fatalf("root must remain: %v", err)
	}
	if _, err := os.Stat(root2); err != nil {
		t.Fatalf("root2 must remain (邻居不应被误删): %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Fatalf("a should be pruned: %v", err)
	}
}

func TestIsSafeFilename(t *testing.T) {
	good := []string{"a.txt", "中文.png", "a b.zip", "a-b_c.md"}
	bad := []string{"", ".", "..", "a/b", "a\\b", "\x00"}
	for _, n := range good {
		if !isSafeFilename(n) {
			t.Errorf("isSafeFilename(%q) = false, want true", n)
		}
	}
	for _, n := range bad {
		if isSafeFilename(n) {
			t.Errorf("isSafeFilename(%q) = true, want false", n)
		}
	}
}

func TestReadmeScore(t *testing.T) {
	cases := map[string]int{
		"README.md":   3,
		"readme.md":   2,
		"README.MD":   1,
		"ReadMe.md":   0,
		"notes.md":    -1,
		"README.html": -1,
	}
	for name, want := range cases {
		if got := readmeScore(name); got != want {
			t.Errorf("readmeScore(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestEscapeURLPath(t *testing.T) {
	cases := map[string]string{
		"":       "/",
		"/":      "/",
		"/a/b":   "/a/b",
		"/a b/c": "/a%20b/c",
		"/中/文":   "/%E4%B8%AD/%E6%96%87",
	}
	for in, want := range cases {
		if got := escapeURLPath(in); got != want {
			t.Errorf("escapeURLPath(%q) = %q, want %q", in, got, want)
		}
	}
}
