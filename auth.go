package main

import (
	"crypto/subtle"
	"log"
	"net/http"
)

// requireAuth 是写/删操作的鉴权中间件，验证 Authorization: Bearer <ADMIN_TOKEN>
func requireAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	want := []byte("Bearer " + token)
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(auth), want) != 1 {
			reason := "invalid token"
			if auth == "" {
				reason = "missing Authorization header"
			}
			log.Printf("auth failed (%s): %s %s", reason, r.Method, r.URL.Path)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}
