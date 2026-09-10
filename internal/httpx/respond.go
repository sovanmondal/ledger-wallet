// Package httpx contains the HTTP transport: router, middleware, and handlers.
package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/sovanmondal/ledger-wallet/internal/obs"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error struct {
		Code          string `json:"code"`
		Message       string `json:"message"`
		CorrelationID string `json:"correlation_id,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = msg
	b.Error.CorrelationID = obs.CorrelationID(r.Context())
	writeJSON(w, status, b)
}
