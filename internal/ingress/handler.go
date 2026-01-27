package ingress

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"event-inbox/internal/crypto"
	"event-inbox/internal/storage"
	"event-inbox/internal/util"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB        *pgxpool.Pool
	S3        *storage.S3
	Encryptor *crypto.Encryptor
	MaxBytes  int64
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Post("/v1/ingress/{provider}/{tenant_id}/{endpoint_id}", h.Ingest)

	return r
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	tenantID := chi.URLParam(r, "tenant_id")
	endpointID := chi.URLParam(r, "endpoint_id")

	if provider == "" || tenantID == "" || endpointID == "" {
		http.Error(w, "missing path params", http.StatusBadRequest)
		return
	}

	body, err := util.ReadBodyLimited(r, h.MaxBytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}

	sum := sha256.Sum256(body)
	bodySHA := hex.EncodeToString(sum[:])

	enc, err := h.Encryptor.Encrypt(body)
	if err != nil {
		http.Error(w, "encrypt failed", http.StatusInternalServerError)
		return
	}

	eventID := uuid.New()
	objectKey := fmt.Sprintf("%s/%s/%s/%s.bin", tenantID, provider, endpointID, eventID.String())

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.S3.Put(ctx, objectKey, bytes.NewReader(enc), int64(len(enc)), "application/octet-stream"); err != nil {
		http.Error(w, "storage failed", http.StatusInternalServerError)
		return
	}

	headersJSON, _ := json.Marshal(r.Header)

	ip := clientIP(r)

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err = h.DB.Exec(ctx, `
		INSERT INTO webhook_events
		  (id, provider, tenant_id, endpoint_id, request_ip, headers_json, content_type, body_size_bytes, payload_object_key, payload_sha256)
		VALUES
		  ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		eventID, provider, tenantID, endpointID,
		ip, string(headersJSON), contentType, len(body),
		objectKey, bodySHA,
	)

	if err != nil {
		http.Error(w, "db insert failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"event_id": eventID.String(),
		"status":   "stored",
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
