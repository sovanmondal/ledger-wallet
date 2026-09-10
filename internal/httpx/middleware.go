package httpx

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/sovanmondal/ledger-wallet/internal/obs"
	"github.com/sovanmondal/ledger-wallet/internal/store"
)

type ctxKey int

const (
	userKey ctxKey = iota
	routeKey
)

func userFrom(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userKey).(store.User)
	return u, ok
}

// withRoute records a low-cardinality route label (e.g. "GET /wallets/{id}") for metrics.
func withRoute(name string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if holder, ok := r.Context().Value(routeKey).(*string); ok {
			*holder = name
		}
		next(w, r)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
		r.ResponseWriter.WriteHeader(code)
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

func (s *Server) recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				obs.L(r.Context()).Error("panic", "recover", rec)
				writeError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) correlationMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get("X-Correlation-ID")
		if cid == "" {
			cid = r.Header.Get("X-Request-Id")
		}
		if cid == "" {
			cid = obs.NewCorrelationID()
		}
		l := s.logger.With("correlation_id", cid, "method", r.Method, "path", r.URL.Path)
		ctx := obs.WithCorrelationID(r.Context(), cid)
		ctx = obs.WithLogger(ctx, l)
		w.Header().Set("X-Correlation-ID", cid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) metricsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := "unknown"
		ctx := context.WithValue(r.Context(), routeKey, &route)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(ctx))
		s.metrics.Observe(route, r.Method, rec.status, time.Since(start))
	})
}

func (s *Server) corsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Correlation-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authMW resolves the bearer token to a user and stores it in the context.
func (s *Server) authMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "missing or malformed bearer token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		u, err := s.store.UserByToken(r.Context(), token)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "invalid bearer token")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
