// Package server provides the implementation of the WebSocket relay server and its middleware.
package server

import (
	"net/http"
	"strings"
)

func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin != "" {
			if s.domain != "" {
				if strings.HasSuffix(origin, s.domain) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else {
					w.Header().Set("Access-Control-Allow-Origin", "https://"+s.domain)
				}
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func (s *Server) cspMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		csp := "default-src 'self'; connect-src 'self' wss://* ws://localhost:*; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';"
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next(w, r)
	}
}

func (s *Server) forceWSSMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.domain == "" {
			next(w, r)
			return
		}

		isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

		if !isHTTPS {
			http.Redirect(w, r, "https://"+s.domain+r.RequestURI, http.StatusMovedPermanently)
			return
		}

		next(w, r)
	}
}

func (s *Server) reverseProxyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
			r.URL.Scheme = forwardedProto
		}

		if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
			r.Host = forwardedHost
		}

		next(w, r)
	}
}
