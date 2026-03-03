package main

import (
	"crypto/subtle"
	"log"
	"net/http"
)

// requireAuth 是写/删操作的鉴权中间件，验证 Authorization: Bearer <ADMIN_TOKEN>
func requireAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		want := "Bearer " + token
		if subtle.ConstantTimeCompare([]byte(auth), []byte(want)) != 1 {
			log.Printf("auth failed: %s %s", r.Method, r.URL.Path)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}
