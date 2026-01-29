-- =========================
-- Webhook events
-- =========================
CREATE TABLE IF NOT EXISTS webhook_events (
id UUID PRIMARY KEY,
provider TEXT NOT NULL,
tenant_id TEXT NOT NULL,
endpoint_id TEXT NOT NULL,

    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_ip TEXT NULL,

    headers_json JSONB NOT NULL,
    content_type TEXT NULL,
    body_size_bytes INTEGER NOT NULL,

    payload_object_key TEXT NOT NULL,
    payload_sha256 TEXT NOT NULL,

    -- NEW: Idempotency support (prevents duplicate events)
    idempotency_key TEXT NULL
    );

CREATE INDEX IF NOT EXISTS idx_webhook_events_tenant_time
    ON webhook_events (tenant_id, received_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_events_endpoint_time
    ON webhook_events (endpoint_id, received_at DESC);

-- unique idempotency constraint per provider+tenant+endpoint
CREATE UNIQUE INDEX IF NOT EXISTS uniq_webhook_idempotency
    ON webhook_events (provider, tenant_id, endpoint_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- =========================
-- Delivery endpoints (customer URLs)
-- =========================
CREATE TABLE IF NOT EXISTS endpoints (
 id UUID PRIMARY KEY,
 tenant_id TEXT NOT NULL,
 endpoint_id TEXT NOT NULL,

  delivery_url TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,

     --signing secret used to sign deliveries (HMAC)
  signing_secret TEXT NULL,

  max_attempts INTEGER NOT NULL DEFAULT 12,
  initial_backoff_seconds INTEGER NOT NULL DEFAULT 5,
  max_backoff_seconds INTEGER NOT NULL DEFAULT 600,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, endpoint_id)
    );

-- =========================
-- Delivery jobs / attempts
-- =========================
CREATE TABLE IF NOT EXISTS deliveries (
id UUID PRIMARY KEY,
event_id UUID NOT NULL REFERENCES webhook_events(id) ON DELETE CASCADE,

    tenant_id TEXT NOT NULL,
    endpoint_id TEXT NOT NULL,

    status TEXT NOT NULL, -- pending, delivering, delivered, failed
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    last_status_code INTEGER NULL,
    last_error TEXT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );

CREATE INDEX IF NOT EXISTS idx_deliveries_ready
    ON deliveries (status, next_attempt_at);

CREATE INDEX IF NOT EXISTS idx_deliveries_event
    ON deliveries (event_id);
