-- +goose Up
-- Indexes for dashboard stats queries that filter logs by created_at
-- and join broadcast_logs with broadcast_task_logs on broadcast_task_log_id.
CREATE INDEX IF NOT EXISTS idx_update_logs_created_at ON update_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_broadcast_task_logs_created_at ON broadcast_task_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_broadcast_logs_created_at ON broadcast_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_broadcast_logs_broadcast_task_log_id ON broadcast_logs(broadcast_task_log_id);
CREATE INDEX IF NOT EXISTS idx_chats_created_at ON chats(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_update_logs_created_at;
DROP INDEX IF EXISTS idx_broadcast_task_logs_created_at;
DROP INDEX IF EXISTS idx_broadcast_logs_created_at;
DROP INDEX IF EXISTS idx_broadcast_logs_broadcast_task_log_id;
DROP INDEX IF EXISTS idx_chats_created_at;