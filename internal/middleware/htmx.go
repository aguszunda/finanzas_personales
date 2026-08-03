package middleware

import (
	"context"
	"net/http"
)

type htmxKey string

const IsHTMXKey htmxKey = "isHTMX"

func DetectHTMX(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isHTMX := r.Header.Get("HX-Request") == "true"
		ctx := context.WithValue(r.Context(), IsHTMXKey, isHTMX)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func IsHTMXRequest(ctx context.Context) bool {
	if v, ok := ctx.Value(IsHTMXKey).(bool); ok {
		return v
	}
	return false
}
