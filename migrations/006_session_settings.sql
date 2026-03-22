-- Migration: 006_session_settings
-- Description: Add settings jsonb column to sessions table for session-specific configs

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS settings JSONB DEFAULT '{}'::jsonb;
