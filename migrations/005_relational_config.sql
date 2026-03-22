-- Migration: 005_relational_config
-- Description: Drop jsonb runtime_config and use structured tables

DROP TABLE IF EXISTS runtime_config;

CREATE TABLE IF NOT EXISTS system_config (
    id INTEGER PRIMARY KEY DEFAULT 1,

    -- Global
    max_task_depth INTEGER DEFAULT 3,
    max_queue_size INTEGER DEFAULT 1000,

    -- LLM
    embedding_dims INTEGER DEFAULT 1024,
    max_tokens INTEGER DEFAULT 4096,
    timeout_seconds INTEGER DEFAULT 120,
    system_prompt TEXT DEFAULT '',
    agent_system_prompt TEXT DEFAULT '',

    -- Agent Pool
    min_agents INTEGER DEFAULT 5,
    max_agents INTEGER DEFAULT 50,
    idle_timeout_seconds INTEGER DEFAULT 300,

    -- Snapshots
    snapshot_enabled BOOLEAN DEFAULT true,
    snapshot_storage_path VARCHAR(255) DEFAULT '/var/catgirl/snapshots',
    snapshot_max_storage_bytes BIGINT DEFAULT 10737418240,
    snapshot_retention_completed VARCHAR(50) DEFAULT '7d',
    snapshot_retention_failed VARCHAR(50) DEFAULT '30d',
    snapshot_retention_exited VARCHAR(50) DEFAULT '7d',
    snapshot_retention_interrupted VARCHAR(50) DEFAULT '14d',

    -- Telegram
    telegram_bot_token VARCHAR(255) DEFAULT '',
    telegram_webhook_url VARCHAR(500) DEFAULT '',
    telegram_listen_addr VARCHAR(255) DEFAULT '',

    -- Auth
    auth_jwt_secret VARCHAR(255) DEFAULT '',
    auth_jwt_issuer VARCHAR(255) DEFAULT 'catgirl-runtime',
    auth_allowed_memberships JSONB DEFAULT '["free", "basic", "pro", "enterprise"]'::jsonb,

    -- Context
    context_max_tokens INTEGER DEFAULT 128000,
    context_compaction_threshold FLOAT DEFAULT 0.8,
    context_preserve_recent_turns INTEGER DEFAULT 20,
    context_compaction_agent_type VARCHAR(50) DEFAULT 'reasoner',

    -- RAG
    rag_enabled BOOLEAN DEFAULT true,
    rag_default_top_k INTEGER DEFAULT 5,
    rag_auto_retrieve_enabled BOOLEAN DEFAULT true,
    rag_auto_retrieve_on_llm_call BOOLEAN DEFAULT true,
    rag_auto_retrieve_top_k INTEGER DEFAULT 3,
    rag_auto_retrieve_max_results INTEGER DEFAULT 10,
    rag_min_similarity FLOAT DEFAULT 0.7,

    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by VARCHAR(255)
);

-- Ensure only one row exists
INSERT INTO system_config (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS llm_providers (
    id SERIAL PRIMARY KEY,
    provider_type VARCHAR(50) NOT NULL, -- 'gp', 'reasoner', 'embedding'
    base_url VARCHAR(500) NOT NULL,
    api_key VARCHAR(255) NOT NULL,
    models JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_llm_providers_type ON llm_providers(provider_type);
