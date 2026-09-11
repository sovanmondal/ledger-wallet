package httpx

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/sovanmondal/ledger-wallet/internal/obs"
	"github.com/sovanmondal/ledger-wallet/internal/store"
)

// Server holds dependencies shared across handlers.
type Server struct {
	store         *store.Store
	metrics       *obs.Metrics
	logger        *slog.Logger
	allowedOrigin string
	ui            fs.FS // embedded static UI, served at "/"
}

// NewServer constructs a Server. ui may be nil (then "/" serves the JSON index).
func NewServer(st *store.Store, m *obs.Metrics, logger *slog.Logger, allowedOrigin string, ui fs.FS) *Server {
	return &Server{store: st, metrics: m, logger: logger, allowedOrigin: allowedOrigin, ui: ui}
}

// Handler builds the routed, middleware-wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /auth/register", withRoute("POST /auth/register", s.handleRegister))
	mux.Handle("POST /wallets", s.authMW(withRoute("POST /wallets", s.handleCreateWallet)))
	mux.Handle("GET /wallets/{id}", s.authMW(withRoute("GET /wallets/{id}", s.handleGetWallet)))
	mux.Handle("POST /wallets/{id}/topup", s.authMW(withRoute("POST /wallets/{id}/topup", s.handleTopup)))
	mux.Handle("POST /transfers", s.authMW(withRoute("POST /transfers", s.handleCreateTransfer)))
	mux.Handle("GET /transfers/{id}", s.authMW(withRoute("GET /transfers/{id}", s.handleGetTransfer)))
	mux.Handle("POST /transfers/{id}/reverse", s.authMW(withRoute("POST /transfers/{id}/reverse", s.handleReverse)))

	mux.HandleFunc("GET /healthz", withRoute("GET /healthz", s.handleHealthz))
	mux.HandleFunc("GET /readyz", withRoute("GET /readyz", s.handleReadyz))
	mux.HandleFunc("GET /api", withRoute("GET /api", s.handleIndex))
	mux.Handle("GET /metrics", s.metrics.Handler())

	// Serve the embedded UI at "/" (and its assets like /styles.css, /app.js). API routes above
	// are more specific and take precedence. Falls back to the JSON index if no UI is embedded.
	if s.ui != nil {
		mux.Handle("GET /", http.FileServer(http.FS(s.ui)))
	} else {
		mux.HandleFunc("GET /{$}", withRoute("GET /", s.handleIndex))
	}

	// Middleware chain, outermost first: recover → correlation → cors → metrics → mux.
	var h http.Handler = mux
	h = s.metricsMW(h)
	h = s.corsMW(h)
	h = s.correlationMW(h)
	h = s.recoverMW(h)
	return h
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "not_ready", "database not reachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "LedgerWallet",
		"status":  "ok",
		"docs":    "https://github.com/sovanmondal/ledger-wallet",
		"endpoints": []string{
			"POST /auth/register",
			"POST /wallets",
			"GET /wallets/{id}",
			"POST /wallets/{id}/topup",
			"POST /transfers",
			"GET /transfers/{id}",
			"POST /transfers/{id}/reverse",
			"GET /healthz",
			"GET /readyz",
			"GET /metrics",
		},
		"note": "JSON API. The operations console UI is served at /",
	})
}
