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