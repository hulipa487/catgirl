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
ON CONFLICT (name) DO NOTHING;