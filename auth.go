package main

import (
	"crypto/subtle"
	"log"
	"net/http"
)

const basicAuthUser = "admin"

// hasValidBearerAuth 验证 Authorization: Bearer <ADMIN_TOKEN>
func hasValidBearerAuth(r *http.Request, token string) bool {
	want := []byte("Bearer " + token)
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) == 1
}

// hasValidBasicAuth 验证 HTTP Basic Auth，账号为 admin，密码为 ADMIN_TOKEN
func hasValidBasicAuth(r *http.Request, token string) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(basicAuthUser))
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(token))
	return (userMatch & passMatch) == 1
}

// requireAuth 是写/删操作的鉴权中间件，验证 Authorization: Bearer <ADMIN_TOKEN>
func requireAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasValidBearerAuth(r, token) {
			reason := "invalid token"
			if r.Header.Get("Authorization") == "" {
				reason = "missing Authorization header"
			}
			log.Printf("auth failed (%s): %s %s", reason, r.Method, r.URL.Path)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

// requireDirListAuth 校验目录列表访问权限，支持 Basic Auth (admin/<ADMIN_TOKEN>) 和 Bearer Token；
// 未通过时返回 401 及 WWW-Authenticate 响应头以触发浏览器登录框
func requireDirListAuth(w http.ResponseWriter, r *http.Request, token string) bool {
	if hasValidBasicAuth(r, token) || hasValidBearerAuth(r, token) {
		return true
	}
	reason := "invalid credentials"
	if r.Header.Get("Authorization") == "" {
		reason = "missing Authorization header"
	}
	log.Printf("dir list auth failed (%s): %s %s", reason, r.Method, r.URL.Path)
	w.Header().Set("WWW-Authenticate", `Basic realm="restfs", charset="UTF-8"`)
	writeError(w, http.StatusUnauthorized, "unauthorized")
	return false
}
