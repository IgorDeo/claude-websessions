package server

import (
	"net/http"
	"strings"
)

func authMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			qToken := r.URL.Query().Get("token")
			bearerToken := ""
			if strings.HasPrefix(auth, "Bearer ") {
				bearerToken = strings.TrimPrefix(auth, "Bearer ")
			}
			if bearerToken != token && qToken != token {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
