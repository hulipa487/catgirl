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
CREATE INDEX IF NOT EXISTS idx_session_turns_timestamp ON session_turns(timestamp);