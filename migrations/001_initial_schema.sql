-- Migration: 001_initial_schema
-- Description: Create core tables for Catgirl Agentic Runtime

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    telegram_user_id BIGINT NOT NULL,
    name VARCHAR(255),
    status VARCHAR(50) DEFAULT 'active',
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
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_task_families_session ON task_families(session_id);
CREATE INDEX IF NOT EXISTS idx_task_families_status ON task_families(status);

-- Task instances table
CREATE TABLE IF NOT EXISTS task_instances (
    instance_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID REFERENCES task_families(task_id) ON DELETE CASCADE,
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    owner_id VARCHAR(255) NOT NULL,
    depth INTEGER NOT NULL DEFAULT 0,
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
    constraints JSONB,
    container_snapshot_id UUID
);

CREATE INDEX IF NOT EXISTS idx_instances_task ON task_instances(task_id);
CREATE INDEX IF NOT EXISTS idx_instances_session ON task_instances(session_id);
CREATE INDEX IF NOT EXISTS idx_instances_owner ON task_instances(owner_id);
CREATE INDEX IF NOT EXISTS idx_instances_status ON task_instances(status);
CREATE INDEX IF NOT EXISTS idx_instances_priority ON task_instances(priority_score DESC);
CREATE INDEX IF NOT EXISTS idx_instances_depth ON task_instances(depth);
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
CREATE INDEX IF NOT EXISTS idx_memories_embedding ON long_term_memories USING ivfflat(embedding cosine_ops);
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
