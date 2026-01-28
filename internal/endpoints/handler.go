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
	DeliveryURL string `json:"delivery_url"`
	Enabled     *bool  `json:"enabled,omitempty"`
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := h.DB.Exec(ctx, `
		INSERT INTO endpoints (id, tenant_id, endpoint_id, delivery_url, enabled)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id, endpoint_id)
		DO UPDATE SET delivery_url = EXCLUDED.delivery_url,
		              enabled = EXCLUDED.enabled,
		              updated_at = now()
	`, uuid.New(), tenantID, endpointID, req.DeliveryURL, enabled)
	if err != nil {
		http.Error(w, "db upsert failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tenant_id":    tenantID,
		"endpoint_id":  endpointID,
		"delivery_url": req.DeliveryURL,
		"enabled":      enabled,
	})
}
