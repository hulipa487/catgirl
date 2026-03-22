-- Migration: 004_runtime_config
-- Description: Create table for storing runtime configuration

CREATE TABLE IF NOT EXISTS runtime_config (
    id SERIAL PRIMARY KEY,
    config JSONB NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by VARCHAR(255)
);

-- Insert initial default config if empty
INSERT INTO runtime_config (id, config)
VALUES (1, '{}'::jsonb)
ON CONFLICT (id) DO NOTHING;
