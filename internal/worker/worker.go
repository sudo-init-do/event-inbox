package worker

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"event-inbox/internal/crypto"
	"event-inbox/internal/storage"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	DB        *pgxpool.Pool
	S3        *storage.S3
	Encryptor *crypto.Encryptor
	Client    *http.Client
}

type job struct {
	DeliveryID uuid.UUID
	EventID    uuid.UUID

	TenantID   string
	EndpointID string

	DeliveryURL string
	ObjectKey   string
	ContentType string

	AttemptCount int
	MaxAttempts  int
	InitialBack  int
	MaxBack      int
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.processOne(ctx)
		}
	}
}

func (w *Worker) processOne(ctx context.Context) error {
	if w.Client == nil {
		w.Client = &http.Client{Timeout: 10 * time.Second}
	}

	// Claim one job safely
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var j job
	err = tx.QueryRow(ctx, `
		WITH next_job AS (
			SELECT d.id
			FROM deliveries d
			WHERE d.status IN ('pending','delivering')
			  AND d.next_attempt_at <= now()
			ORDER BY d.next_attempt_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		SELECT
			d.id,
			d.event_id,
			d.tenant_id,
			d.endpoint_id,
			e.delivery_url,
			we.payload_object_key,
			we.content_type,
			d.attempt_count,
			e.max_attempts,
			e.initial_backoff_seconds,
			e.max_backoff_seconds
		FROM deliveries d
		JOIN next_job nj ON nj.id = d.id
		JOIN endpoints e ON e.tenant_id = d.tenant_id AND e.endpoint_id = d.endpoint_id
		JOIN webhook_events we ON we.id = d.event_id
		WHERE e.enabled = true
	`).Scan(
		&j.DeliveryID,
		&j.EventID,
		&j.TenantID,
		&j.EndpointID,
		&j.DeliveryURL,
		&j.ObjectKey,
		&j.ContentType,
		&j.AttemptCount,
		&j.MaxAttempts,
		&j.InitialBack,
		&j.MaxBack,
	)

	if err != nil {
		// No job ready is normal
		_ = tx.Commit(ctx)
		return nil
	}

	_, _ = tx.Exec(ctx, `
		UPDATE deliveries
		SET status='delivering', updated_at=now()
		WHERE id=$1
	`, j.DeliveryID)

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Fetch encrypted payload
	getCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	encBlob, err := w.S3.Get(getCtx, j.ObjectKey)
	cancel()
	if err != nil {
		return w.failAndSchedule(ctx, j, 0, fmt.Sprintf("s3 get failed: %v", err))
	}

	// Decrypt
	plain, err := w.Encryptor.Decrypt(encBlob)
	if err != nil {
		return w.failAndSchedule(ctx, j, 0, fmt.Sprintf("decrypt failed: %v", err))
	}

	// Forward exact bytes
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.DeliveryURL, bytes.NewReader(plain))
	if err != nil {
		return w.failAndSchedule(ctx, j, 0, fmt.Sprintf("build request: %v", err))
	}

	if j.ContentType != "" {
		req.Header.Set("Content-Type", j.ContentType)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	req.Header.Set("X-Event-Inbox-Event-ID", j.EventID.String())
	req.Header.Set("X-Event-Inbox-Delivery-ID", j.DeliveryID.String())
	req.Header.Set("X-Event-Inbox-Tenant-ID", j.TenantID)
	req.Header.Set("X-Event-Inbox-Endpoint-ID", j.EndpointID)

	resp, err := w.Client.Do(req)
	if err != nil {
		return w.failAndSchedule(ctx, j, 0, fmt.Sprintf("request failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = w.DB.Exec(ctx, `
			UPDATE deliveries
			SET status='delivered',
			    last_status_code=$2,
			    last_error=NULL,
			    updated_at=now()
			WHERE id=$1
		`, j.DeliveryID, resp.StatusCode)
		return nil
	}

	return w.failAndSchedule(ctx, j, resp.StatusCode, fmt.Sprintf("non-2xx: %d", resp.StatusCode))
}

func (w *Worker) failAndSchedule(ctx context.Context, j job, statusCode int, errMsg string) error {
	attempt := j.AttemptCount + 1
	maxAttempts := j.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 12
	}

	initial := j.InitialBack
	if initial <= 0 {
		initial = 5
	}
	maxBack := j.MaxBack
	if maxBack <= 0 {
		maxBack = 600
	}

	// backoff = min(initial * 2^(attempt-1), maxBack)
	backoff := initial * (1 << min(attempt-1, 10))
	if backoff > maxBack {
		backoff = maxBack
	}

	next := time.Now().Add(time.Duration(backoff) * time.Second)

	if attempt >= maxAttempts {
		_, _ = w.DB.Exec(ctx, `
			UPDATE deliveries
			SET status='failed',
			    attempt_count=$2,
			    last_status_code=$3,
			    last_error=$4,
			    updated_at=now()
			WHERE id=$1
		`, j.DeliveryID, attempt, nullIfZero(statusCode), errMsg)
		return nil
	}

	_, _ = w.DB.Exec(ctx, `
		UPDATE deliveries
		SET status='pending',
		    attempt_count=$2,
		    next_attempt_at=$3,
		    last_status_code=$4,
		    last_error=$5,
		    updated_at=now()
		WHERE id=$1
	`, j.DeliveryID, attempt, next, nullIfZero(statusCode), errMsg)

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func nullIfZero(v int) any {
	if v == 0 {
		return nil
	}
	return v
}
