package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sovanmondal/ledger-wallet/internal/obs"
	"github.com/sovanmondal/ledger-wallet/internal/store"
)

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// POST /auth/register  (public bootstrap)
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Username) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "username is required")
		return
	}
	u, err := s.store.RegisterUser(r.Context(), strings.TrimSpace(req.Username))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not register user")
		return
	}
	obs.L(r.Context()).Info("user.registered", "user_id", u.ID, "username", u.Username)
	writeJSON(w, http.StatusOK, u)
}

// POST /wallets  → get-or-create the caller's wallet
func (s *Server) handleCreateWallet(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	wallet, created, err := s.store.GetOrCreateWallet(r.Context(), u.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not get or create wallet")
		return
	}
	if created {
		s.metrics.WalletsCreated.Inc()
		obs.L(r.Context()).Info("wallet.created", "wallet_id", wallet.ID, "user_id", u.ID)
	} else {
		obs.L(r.Context()).Info("wallet.get_or_create.existing", "wallet_id", wallet.ID, "user_id", u.ID)
	}
	writeJSON(w, http.StatusOK, wallet)
}

// GET /wallets/{id}
func (s *Server) handleGetWallet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wallet, err := s.store.GetWallet(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "not_found", "wallet not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not read wallet")
		return
	}
	writeJSON(w, http.StatusOK, wallet)
}

// POST /wallets/{id}/topup  → fund a wallet from the treasury (test seeding)
func (s *Server) handleTopup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		AmountPaise int64 `json:"amount_paise"`
	}
	if err := decode(r, &req); err != nil || req.AmountPaise <= 0 {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "amount_paise must be a positive integer")
		return
	}
	if _, err := s.store.GetWallet(r.Context(), id); err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "wallet not found")
		return
	}
	t, wallet, err := s.store.Topup(r.Context(), id, req.AmountPaise)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not top up wallet")
		return
	}
	obs.L(r.Context()).Info("wallet.topup", "wallet_id", id, "amount_paise", req.AmountPaise, "transfer_id", t.ID, "status", t.Status)
	writeJSON(w, http.StatusOK, map[string]any{
		"wallet":      wallet,
		"transfer_id": t.ID,
		"status":      t.Status,
	})
}

// POST /transfers
func (s *Server) handleCreateTransfer(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req struct {
		From           string `json:"from"`
		To             string `json:"to"`
		AmountPaise    int64  `json:"amount_paise"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "malformed JSON body")
		return
	}
	if req.From == "" || req.To == "" || strings.TrimSpace(req.IdempotencyKey) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "from, to and idempotency_key are required")
		return
	}
	if req.AmountPaise <= 0 {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "amount_paise must be a positive integer")
		return
	}
	if req.From == req.To {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "from and to must differ")
		return
	}

	// Ownership: the caller must own the source wallet.
	fromWallet, err := s.store.GetWallet(r.Context(), req.From)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "not_found", "from wallet not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not read wallet")
		return
	}
	if fromWallet.UserID != u.ID {
		writeError(w, r, http.StatusForbidden, "forbidden", "caller does not own the source wallet")
		return
	}

	t, replay, err := s.store.CreateTransfer(r.Context(), req.IdempotencyKey, req.From, req.To, req.AmountPaise)
	if errors.Is(err, store.ErrConflict) {
		s.metrics.Conflicts.Inc()
		obs.L(r.Context()).Warn("transfer.conflict", "idempotency_key", req.IdempotencyKey)
		writeError(w, r, http.StatusConflict, "conflict", "idempotency key reused with a different request body")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not process transfer")
		return
	}
	s.recordTransferOutcome(r, t, replay, false)
	writeJSON(w, http.StatusOK, t)
}

// GET /transfers/{id}
func (s *Server) handleGetTransfer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.store.GetTransfer(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "not_found", "transfer not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not read transfer")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// POST /transfers/{id}/reverse
func (s *Server) handleReverse(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.IdempotencyKey) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "idempotency_key is required")
		return
	}

	orig, err := s.store.GetTransfer(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "not_found", "transfer not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not read transfer")
		return
	}
	// Ownership: only the original sender may initiate a refund.
	fromWallet, err := s.store.GetWallet(r.Context(), orig.FromWallet)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not read wallet")
		return
	}
	if fromWallet.UserID != u.ID {
		writeError(w, r, http.StatusForbidden, "forbidden", "caller does not own the original source wallet")
		return
	}

	t, replay, err := s.store.ReverseTransfer(r.Context(), id, req.IdempotencyKey)
	switch {
	case errors.Is(err, store.ErrAlreadyReversed):
		s.metrics.ReversalsAlreadyDone.Inc()
		obs.L(r.Context()).Warn("reversal.already_reversed", "original_transfer_id", id)
		writeError(w, r, http.StatusConflict, "already_reversed", "transfer has already been reversed")
		return
	case errors.Is(err, store.ErrNotReversible):
		writeError(w, r, http.StatusConflict, "not_reversible", "only a completed transfer can be reversed")
		return
	case errors.Is(err, store.ErrConflict):
		s.metrics.Conflicts.Inc()
		writeError(w, r, http.StatusConflict, "conflict", "idempotency key reused with a different request body")
		return
	case err != nil:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "could not process reversal")
		return
	}
	s.recordTransferOutcome(r, t, replay, true)
	writeJSON(w, http.StatusOK, t)
}

// recordTransferOutcome emits domain logs and increments the right counters.
func (s *Server) recordTransferOutcome(r *http.Request, t store.Transfer, replay, reversal bool) {
	l := obs.L(r.Context())
	if replay {
		s.metrics.IdempotentReplays.Inc()
		l.Info("transfer.idempotent_replay", "transfer_id", t.ID, "kind", t.Kind, "status", t.Status)
		return
	}
	switch t.Status {
	case "completed":
		if reversal {
			s.metrics.ReversalsCreated.Inc()
			l.Info("reversal.created", "transfer_id", t.ID, "reverses", derefStr(t.ReversesTransferID), "amount_paise", t.AmountPaise)
		} else {
			s.metrics.TransfersCreated.Inc()
			l.Info("transfer.created", "transfer_id", t.ID, "amount_paise", t.AmountPaise)
		}
		l.Info("transfer.debited", "transfer_id", t.ID, "wallet_id", t.FromWallet, "amount_paise", t.AmountPaise)
		l.Info("transfer.credited", "transfer_id", t.ID, "wallet_id", t.ToWallet, "amount_paise", t.AmountPaise)
	case "declined":
		reason := derefStr(t.DeclineReason)
		if reason == "insufficient_funds" {
			if reversal {
				s.metrics.ReversalsDeclined.Inc()
			} else {
				s.metrics.TransfersDeclinedFunds.Inc()
			}
			l.Info("transfer.declined_insufficient_funds", "transfer_id", t.ID, "reason", reason)
		} else {
			s.metrics.TransfersDeclinedOther.Inc()
			l.Info("transfer.declined", "transfer_id", t.ID, "reason", reason)
		}
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
