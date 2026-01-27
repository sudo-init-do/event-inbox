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
  payload_sha256 TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_tenant_time
  ON webhook_events (tenant_id, received_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_events_endpoint_time
  ON webhook_events (endpoint_id, received_at DESC);
