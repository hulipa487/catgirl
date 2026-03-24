-- Migration: 001_initial_schema
-- Description: Create core tables for Catgirl Agentic Runtime

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    telegram_user_id BIGINT NOT NULL,
    bot_token VARCHAR(255) NOT NULL DEFAULT '',
    name VARCHAR(255),
    status VARCHAR(50) DEFAULT 'active',
    settings JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    orchestrator_state JSONB,
    metadata JSONB
);

-- Telegram users table
CREATE TABLE IF NOT EXISTS telegram_users (
    telegram_user_id BIGINT PRIMARY KEY,
    session_id UUID REFERENCES sessions(id) ON DELETE SET NULL,
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    is_banned BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_activity TIMESTAMPTZ DEFAULT NOW()
);

-- Note: In PostgreSQL, adding a foreign key to an existing table using IF NOT EXISTS isn't natively supported in a clean way via standard syntax, so we just set up the basic references above.

CREATE INDEX IF NOT EXISTS idx_telegram_users_session ON telegram_users(session_id);
CREATE INDEX IF NOT EXISTS idx_sessions_telegram_user ON sessions(telegram_user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);

-- Task families table
CREATE TABLE IF NOT EXISTS task_families (
    task_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    container_id VARCHAR(255),
    root_description TEXT NOT NULL,
    status VARCHAR(50) DEFAULT 'in_progress',
    max_depth_reached INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    container_snapshot_id UUID
);

CREATE INDEX IF NOT EXISTS idx_task_families_session ON task_families(session_id);
CREATE INDEX IF NOT EXISTS idx_task_families_status ON task_families(status);

-- Task instances table
CREATE TABLE IF NOT EXISTS task_instances (
    instance_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID REFERENCES task_families(task_id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    agent_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    priority VARCHAR(20) DEFAULT 'normal',
    priority_score FLOAT DEFAULT 0,
    assigned_agent_id VARCHAR(255),
    parent_instance_id UUID REFERENCES task_instances(instance_id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    result JSONB,
    error TEXT,
    constraints JSONB
);

CREATE INDEX IF NOT EXISTS idx_instances_task ON task_instances(task_id);
CREATE INDEX IF NOT EXISTS idx_instances_status ON task_instances(status);
CREATE INDEX IF NOT EXISTS idx_instances_priority ON task_instances(priority_score DESC);
CREATE INDEX IF NOT EXISTS idx_instances_agent_type ON task_instances(agent_type);

-- Container snapshots table
CREATE TABLE IF NOT EXISTS container_snapshots (
    snapshot_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID REFERENCES task_families(task_id) ON DELETE SET NULL,
    instance_id UUID REFERENCES task_instances(instance_id) ON DELETE SET NULL,
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    container_id VARCHAR(255) NOT NULL,
    image_id VARCHAR(255) NOT NULL,
    image_name VARCHAR(255),
    reason VARCHAR(50) NOT NULL,
    volumes JSONB,
    environment JSONB,
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_snapshots_task ON container_snapshots(task_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_session ON container_snapshots(session_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_reason ON container_snapshots(reason);
CREATE INDEX IF NOT EXISTS idx_snapshots_expires ON container_snapshots(expires_at);

-- Agents table (global pool)
CREATE TABLE IF NOT EXISTS agents (
    id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'idle',
    current_instance_id UUID REFERENCES task_instances(instance_id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_active_at TIMESTAMPTZ,
    tasks_completed INTEGER DEFAULT 0,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_type ON agents(type);

-- Working memory table (per-agent)
CREATE TABLE IF NOT EXISTS working_memory (
    agent_id VARCHAR(255) NOT NULL,
    key VARCHAR(255) NOT NULL,
    value JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (agent_id, key)
);

CREATE INDEX IF NOT EXISTS idx_working_memory_agent ON working_memory(agent_id);

-- Long-term memories table
CREATE TABLE IF NOT EXISTS long_term_memories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    tier VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    embedding VECTOR(1024),
    metadata JSONB,
    access_count INTEGER DEFAULT 0,
    last_accessed_at TIMESTAMPTZ,
    source_agent_ids JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_memories_session ON long_term_memories(session_id);
CREATE INDEX IF NOT EXISTS idx_memories_tier ON long_term_memories(tier);
-- Note: removed vector index due to extension operator class issues
-- CREATE INDEX IF NOT EXISTS idx_memories_embedding ON long_term_memories USING ivfflat(embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS idx_memories_access ON long_term_memories(access_count DESC, last_accessed_at);
CREATE INDEX IF NOT EXISTS idx_memories_expires ON long_term_memories(expires_at);

-- Skills table
CREATE TABLE IF NOT EXISTS skills (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    version VARCHAR(50) DEFAULT '1.0.0',
    definition JSONB NOT NULL,
    code TEXT,
    created_by_agent_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(session_id, name)
);

CREATE INDEX IF NOT EXISTS idx_skills_session ON skills(session_id);

-- MCP servers table
CREATE TABLE IF NOT EXISTS mcp_servers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    connection_type VARCHAR(50),
    connection_string TEXT,
    command VARCHAR(500),
    status VARCHAR(50) DEFAULT 'disconnected',
    tools JSONB,
    created_by_agent_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(session_id, name)
);

CREATE INDEX IF NOT EXISTS idx_mcp_servers_session ON mcp_servers(session_id);

-- Usage records table
CREATE TABLE IF NOT EXISTS usage_records (
    usage_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID REFERENCES task_instances(instance_id) ON DELETE SET NULL,
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    user_id VARCHAR(255),
    operation_type VARCHAR(50) NOT NULL,
    operation_name VARCHAR(255),
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    membership_level VARCHAR(50),
    cost_multiplier FLOAT DEFAULT 1.0,
    effective_tokens FLOAT DEFAULT 0,
    timestamp TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_task ON usage_records(task_id);
CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_records(session_id);
CREATE INDEX IF NOT EXISTS idx_usage_user ON usage_records(user_id);
CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_records(timestamp);

-- Task owner channels table
CREATE TABLE IF NOT EXISTS task_owner_channels (
    channel_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_instance_id UUID REFERENCES task_instances(instance_id) ON DELETE CASCADE,
    owner_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_activity TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channels_task ON task_owner_channels(task_instance_id);
CREATE INDEX IF NOT EXISTS idx_channels_owner ON task_owner_channels(owner_id);

-- Task messages table
CREATE TABLE IF NOT EXISTS task_messages (
    message_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel_id UUID REFERENCES task_owner_channels(channel_id) ON DELETE CASCADE,
    from_id VARCHAR(255) NOT NULL,
    to_id VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    read BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_messages_channel ON task_messages(channel_id);
CREATE INDEX IF NOT EXISTS idx_messages_unread ON task_messages(to_id, read);
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON task_messages(timestamp);

-- Conversation history table
CREATE TABLE IF NOT EXISTS conversation_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    turn_id INTEGER NOT NULL,
    thought TEXT,
    action TEXT,
    result JSONB,
    tokens INTEGER DEFAULT 0,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(session_id, turn_id)
);

CREATE INDEX IF NOT EXISTS idx_history_session ON conversation_history(session_id);
CREATE INDEX IF NOT EXISTS idx_history_turn ON conversation_history(session_id, turn_id DESC);


-- Migrate migration tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMPTZ DEFAULT NOW()
);
-- Migration: 002_tools_schema
-- Description: Create tools table for dynamic tool loading

-- Tools table
CREATE TABLE IF NOT EXISTS tools (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    parameters JSONB NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tools_name ON tools(name);
CREATE INDEX IF NOT EXISTS idx_tools_active ON tools(is_active);

-- Seed data: SEND_MESSAGE tool
INSERT INTO tools (name, description, parameters) VALUES
(
    'SEND_MESSAGE',
    'Send a message to the user/orchestrator',
    '{
        "type": "object",
        "properties": {
            "message": {
                "type": "string",
                "description": "The message text to send to the user"
            }
        },
        "required": ["message"]
    }'::jsonb
)
ON CONFLICT (name) DO NOTHING;

-- Seed data: SPAWN_TASK tool
INSERT INTO tools (name, description, parameters) VALUES
(
    'SPAWN_TASK',
    'Spawn a background task to be executed by a worker agent',
    '{
        "type": "object",
        "properties": {
            "description": {
                "type": "string",
                "description": "The task description/prompt for the worker agent to execute"
            }
        },
        "required": ["description"]
    }'::jsonb
)
ON CONFLICT (name) DO NOTHING;

-- Seed data: SET_STATE tool (for worker agents to signal state)
INSERT INTO tools (name, description, parameters) VALUES
(
    'SET_STATE',
    'Set the agent state. BLOCKING = waiting for async tool result, COMPLETED = task finished successfully, FAILED = task failed',
    '{
        "type": "object",
        "properties": {
            "state": {
                "type": "string",
                "enum": ["BLOCKING", "COMPLETED", "FAILED"],
                "description": "The state to set: BLOCKING (waiting for async result), COMPLETED (task finished), FAILED (task failed)"
            },
            "result": {
                "type": "string",
                "description": "Result summary or error reason for COMPLETED or FAILED states"
            }
        },
        "required": ["state"]
    }'::jsonb
)
ON CONFLICT (name) DO NOTHING;

-- Seed data: SEND_PARENT tool
INSERT INTO tools (name, description, parameters) VALUES
(
    'SEND_PARENT',
    'Send a message to your parent task/orchestrator to ask for clarification, report partial results, or inform about blockers.',
    '{
        "type": "object",
        "properties": {
            "message": {
                "type": "string",
                "description": "The message to send to the parent task or orchestrator"
            }
        },
        "required": ["message"]
    }'::jsonb
)
ON CONFLICT (name) DO NOTHING;-- Migration: 003_refactor_task_models
-- Description: Update task models to remove redundant fields

-- Add container_snapshot_id to task_families
ALTER TABLE task_families ADD COLUMN IF NOT EXISTS container_snapshot_id UUID REFERENCES container_snapshots(snapshot_id) ON DELETE SET NULL;

-- Remove redundant fields from task_instances
ALTER TABLE task_instances DROP COLUMN IF EXISTS session_id CASCADE;
ALTER TABLE task_instances DROP COLUMN IF EXISTS owner_id CASCADE;
ALTER TABLE task_instances DROP COLUMN IF EXISTS depth CASCADE;
ALTER TABLE task_instances DROP COLUMN IF EXISTS container_snapshot_id CASCADE;
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
    default_orchestrator_tools JSONB DEFAULT '["SEND_MESSAGE", "SPAWN_TASK"]'::jsonb,
    default_agent_tools JSONB DEFAULT '["SET_STATE", "SEND_PARENT"]'::jsonb,

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
    telegram_bots JSONB DEFAULT '[]'::jsonb,
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
-- Migration: 006_session_settings
-- Description: Add settings jsonb column to sessions table for session-specific configs

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS settings JSONB DEFAULT '{}'::jsonb;
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
-- Migration: Worker Context Storage
-- Description: Creates tables for storing worker and orchestrator conversation history
-- Also adds global config fields for tools and docker

-- Add new columns to system_config
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS tools_dir VARCHAR(255) DEFAULT '/var/catgirl/tools';
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS docker_registry VARCHAR(255) DEFAULT '';
ALTER TABLE system_config ADD COLUMN IF NOT EXISTS docker_image VARCHAR(255) DEFAULT 'catgirl-runtime:latest';

-- Create table for worker conversation history
CREATE TABLE IF NOT EXISTS task_instance_turns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    turn_id INTEGER NOT NULL,
    instance_id UUID NOT NULL REFERENCES task_instances(instance_id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL, -- 'user', 'assistant', 'tool'
    content TEXT,
    tool_call_id VARCHAR(100),
    tool_name VARCHAR(100),
    tool_arguments JSONB,
    tool_result JSONB,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(instance_id, turn_id)
);

-- Create table for orchestrator conversation history
CREATE TABLE IF NOT EXISTS session_turns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    turn_id INTEGER NOT NULL,
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL, -- 'user', 'assistant', 'tool'
    content TEXT,
    tool_call_id VARCHAR(100),
    tool_name VARCHAR(100),
    tool_arguments JSONB,
    tool_result JSONB,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(session_id, turn_id)
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_task_instance_turns_instance ON task_instance_turns(instance_id);
CREATE INDEX IF NOT EXISTS idx_task_instance_turns_timestamp ON task_instance_turns(timestamp);
CREATE INDEX IF NOT EXISTS idx_session_turns_session ON session_turns(session_id);
CREATE INDEX IF NOT EXISTS idx_session_turns_timestamp ON session_turns(timestamp);-- Migration: Telegram Bots Table
-- Description: Creates a dedicated table for telegram bots instead of JSONB array

CREATE TABLE IF NOT EXISTS telegram_bots (
    bot_token VARCHAR(255) PRIMARY KEY,
    webhook_url VARCHAR(500) NOT NULL DEFAULT '',
    orchestrator_system_prompt TEXT NOT NULL DEFAULT '',
    agent_system_prompt TEXT NOT NULL DEFAULT '',
    allowed_orchestrator_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_agent_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
    gp_model VARCHAR(100) NOT NULL DEFAULT '',
    reasoner_model VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Migrate existing data from system_config if available
DO $$
DECLARE
    bots_json JSONB;
    bot_element JSONB;
BEGIN
    SELECT telegram_bots INTO bots_json FROM system_config WHERE id = 1;
    
    IF bots_json IS NOT NULL AND jsonb_typeof(bots_json) = 'array' THEN
        FOR bot_element IN SELECT * FROM jsonb_array_elements(bots_json)
        LOOP
            INSERT INTO telegram_bots (
                bot_token, 
                webhook_url, 
                orchestrator_system_prompt, 
                agent_system_prompt, 
                allowed_orchestrator_tools, 
                allowed_agent_tools, 
                gp_model, 
                reasoner_model
            ) VALUES (
                bot_element->>'bot_token',
                COALESCE(bot_element->>'webhook_url', ''),
                COALESCE(bot_element->>'orchestrator_system_prompt', ''),
                COALESCE(bot_element->>'agent_system_prompt', ''),
                COALESCE(bot_element->'allowed_orchestrator_tools', '[]'::jsonb),
                COALESCE(bot_element->'allowed_agent_tools', '[]'::jsonb),
                COALESCE(bot_element->>'gp_model', ''),
                COALESCE(bot_element->>'reasoner_model', '')
            ) ON CONFLICT (bot_token) DO NOTHING;
        END LOOP;
    END IF;
END $$;