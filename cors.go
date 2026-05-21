package main

import "net/http"

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, HEAD, PUT, POST, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Range, If-None-Match, If-Modified-Since")
		h.Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range, Content-Disposition, ETag, Last-Modified")
		h.Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
