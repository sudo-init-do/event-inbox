package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB *pgxpool.Pool
}

type upsertReq struct {
	DeliveryURL    string `json:"delivery_url"`
	Enabled        *bool  `json:"enabled,omitempty"`
	SigningSecret  string `json:"signing_secret,omitempty"`
	MaxAttempts    *int   `json:"max_attempts,omitempty"`
	InitialBackoff *int   `json:"initial_backoff_seconds,omitempty"`
	MaxBackoff     *int   `json:"max_backoff_seconds,omitempty"`
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/v1/endpoints/{tenant_id}/{endpoint_id}", h.UpsertEndpoint)
	return r
}

func (h *Handler) UpsertEndpoint(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	endpointID := chi.URLParam(r, "endpoint_id")
	if tenantID == "" || endpointID == "" {
		http.Error(w, "missing path params", http.StatusBadRequest)
		return
	}

	var req upsertReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeliveryURL == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	maxAttempts := 12
	if req.MaxAttempts != nil && *req.MaxAttempts > 0 {
		maxAttempts = *req.MaxAttempts
	}

	initial := 5
	if req.InitialBackoff != nil && *req.InitialBackoff > 0 {
		initial = *req.InitialBackoff
	}

	maxBack := 600
	if req.MaxBackoff != nil && *req.MaxBackoff > 0 {
		maxBack = *req.MaxBackoff
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := h.DB.Exec(ctx, `
		INSERT INTO endpoints (id, tenant_id, endpoint_id, delivery_url, enabled, signing_secret, max_attempts, initial_backoff_seconds, max_backoff_seconds)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (tenant_id, endpoint_id)
		DO UPDATE SET
			delivery_url = EXCLUDED.delivery_url,
			enabled = EXCLUDED.enabled,
			signing_secret = CASE
				WHEN EXCLUDED.signing_secret = '' THEN endpoints.signing_secret
				ELSE EXCLUDED.signing_secret
			END,
			max_attempts = EXCLUDED.max_attempts,
			initial_backoff_seconds = EXCLUDED.initial_backoff_seconds,
			max_backoff_seconds = EXCLUDED.max_backoff_seconds,
			updated_at = now()
	`, uuid.New(), tenantID, endpointID, req.DeliveryURL, enabled, req.SigningSecret, maxAttempts, initial, maxBack)
	if err != nil {
		http.Error(w, "db upsert failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tenant_id":               tenantID,
		"endpoint_id":             endpointID,
		"delivery_url":            req.DeliveryURL,
		"enabled":                 enabled,
		"max_attempts":            maxAttempts,
		"initial_backoff_seconds": initial,
		"max_backoff_seconds":     maxBack,
		"signing_secret_set":      req.SigningSecret != "",
	})
}
