-- Migration: 003_refactor_task_models
-- Description: Update task models to remove redundant fields

-- Add container_snapshot_id to task_families
ALTER TABLE task_families ADD COLUMN IF NOT EXISTS container_snapshot_id UUID REFERENCES container_snapshots(snapshot_id) ON DELETE SET NULL;

-- Remove redundant fields from task_instances
ALTER TABLE task_instances DROP COLUMN IF EXISTS session_id CASCADE;
ALTER TABLE task_instances DROP COLUMN IF EXISTS owner_id CASCADE;
ALTER TABLE task_instances DROP COLUMN IF EXISTS depth CASCADE;
ALTER TABLE task_instances DROP COLUMN IF EXISTS container_snapshot_id CASCADE;
