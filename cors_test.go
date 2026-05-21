package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()

	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &handler{dataDir: dataDir, token: "secret"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{path...}", h.handleGet)
	mux.HandleFunc("PUT /{path...}", requireAuth("secret", h.handlePut))
	mux.HandleFunc("POST /{path...}", requireAuth("secret", h.handlePost))
	mux.HandleFunc("DELETE /{path...}", requireAuth("secret", h.handleDelete))

	return withCORS(mux)
}

func TestCORSHeadersOnGet(t *testing.T) {
	server := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := rr.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "Content-Range") {
		t.Fatalf("Access-Control-Expose-Headers = %q, want Content-Range", got)
	}
}

func TestCORSPreflightDoesNotRequireAuth(t *testing.T) {
	server := testServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/file.txt", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPut) {
		t.Fatalf("Access-Control-Allow-Methods = %q, want PUT", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want Authorization", got)
	}
}

func TestCORSHeadersOnUnauthorizedWrite(t *testing.T) {
	server := testServer(t)
	req := httptest.NewRequest(http.MethodPut, "/file.txt", strings.NewReader("updated"))
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}
