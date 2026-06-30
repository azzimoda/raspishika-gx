-- DROP

DROP TABLE IF EXISTS update_logs;

DROP INDEX IF EXISTS idx_update_logs_chat_id;

DROP INDEX IF EXISTS idx_update_logs_created_at;

DROP TABLE IF EXISTS sending_logs;

DROP INDEX IF EXISTS idx_sending_log_kind;

DROP INDEX IF EXISTS idx_sending_log_created_at;

-- CREATE

CREATE TABLE IF NOT EXISTS update_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  kind TEXT NOT NULL,
  message_id INTEGER NOT NULL,
  data TEXT NOT NULL,
  elapsed INTEGER NOT NULL, -- milliseconds
  error TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_update_logs_chat_id ON update_logs (chat_id);

CREATE INDEX IF NOT EXISTS idx_update_logs_kind ON update_logs (kind);

CREATE TABLE IF NOT EXISTS broadcast_task_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  elapsed INTEGER NOT NULL, -- milliseconds
  created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_broadcast_task_logs_kind ON broadcast_task_logs (
  kind
);

CREATE TABLE IF NOT EXISTS broadcast_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  broadcast_task_log_id INTEGER NOT NULL REFERENCES broadcast_task_logs(id),
  chat_id INTEGER NOT NULL,
  error TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_broadcast_logs_chat_id ON broadcast_logs (
  chat_id
);
