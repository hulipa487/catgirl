-- Migration: 007_fix_system_config
-- Description: Add missing columns to system_config table

-- Add missing Telegram columns
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS telegram_bots JSONB DEFAULT '[]'::jsonb;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS telegram_listen_addr VARCHAR(255) DEFAULT '';

-- Add missing Auth columns
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS auth_jwt_secret VARCHAR(255) DEFAULT '';
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS auth_jwt_issuer VARCHAR(255) DEFAULT 'catgirl-runtime';
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS auth_allowed_memberships JSONB DEFAULT '["free", "basic", "pro", "enterprise"]'::jsonb;

-- Add missing Context columns
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS context_max_tokens INTEGER DEFAULT 128000;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS context_compaction_threshold FLOAT DEFAULT 0.8;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS context_preserve_recent_turns INTEGER DEFAULT 20;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS context_compaction_agent_type VARCHAR(50) DEFAULT 'reasoner';

-- Add missing RAG columns
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS rag_enabled BOOLEAN DEFAULT true;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS rag_default_top_k INTEGER DEFAULT 5;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS rag_auto_retrieve_enabled BOOLEAN DEFAULT true;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS rag_auto_retrieve_on_llm_call BOOLEAN DEFAULT true;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS rag_auto_retrieve_top_k INTEGER DEFAULT 3;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS rag_auto_retrieve_max_results INTEGER DEFAULT 10;
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS rag_min_similarity FLOAT DEFAULT 0.7;
