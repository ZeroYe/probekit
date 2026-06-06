package mcp

import (
	"net/http"
	"strings"
	"sync"
)

type APIKeyAuth struct {
	keys map[string]struct{}
	mu   sync.RWMutex
}

func NewAPIKeyAuth(keys []string) *APIKeyAuth {
	a := &APIKeyAuth{
		keys: make(map[string]struct{}, len(keys)),
	}
	for _, k := range keys {
		a.keys[k] = struct{}{}
	}
	return a
}

func (a *APIKeyAuth) UpdateKeys(keys []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	newKeys := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		newKeys[k] = struct{}{}
	}
	a.keys = newKeys
}

func (a *APIKeyAuth) Validate(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	key := strings.TrimPrefix(auth, "Bearer ")

	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.keys[key]
	return ok
}

func AuthMiddleware(next http.Handler, auth *APIKeyAuth) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.Validate(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized: invalid or missing api key"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
