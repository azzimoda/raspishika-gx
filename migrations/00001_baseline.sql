-- +goose Up
-- Baseline: final schema state after legacy migrations 0..7.
-- Idempotent so it is safe to apply on already-initialized databases.

CREATE TABLE IF NOT EXISTS chats(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	tg_chat_id INTEGER NOT NULL UNIQUE,
	username TEXT,
	state TEXT DEFAULT 'default',
	department TEXT,
	"group" TEXT,
	daily_sending_time TEXT,
	pair_sending BOOLEAN NOT NULL DEFAULT 0,
	update_notification BOOLEAN NOT NULL DEFAULT 0,
	dark_mode BOOLEAN NOT NULL DEFAULT 0,
	access INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_chats_tg_chat_id ON chats(tg_chat_id);
CREATE INDEX IF NOT EXISTS idx_chats_group ON chats("group");
CREATE INDEX IF NOT EXISTS idx_chats_daily_sending_time ON chats(daily_sending_time);

CREATE TABLE IF NOT EXISTS schedules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cache_key TEXT NOT NULL UNIQUE,
	data TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_schedules_cache_key ON schedules(cache_key);

CREATE TABLE IF NOT EXISTS update_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  kind TEXT NOT NULL,
  message_id INTEGER NOT NULL,
  data TEXT NOT NULL,
  elapsed INTEGER NOT NULL, -- milliseconds
  error TEXT,
  group_or_teacher TEXT NOT NULL DEFAULT '',
  cached BOOLEAN NOT NULL DEFAULT 0, -- True if schedule sent from cache
  created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_update_logs_chat_id ON update_logs (chat_id);
CREATE INDEX IF NOT EXISTS idx_update_logs_kind ON update_logs (kind);

CREATE TABLE IF NOT EXISTS broadcast_task_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  elapsed INTEGER NOT NULL, -- milliseconds
  groups INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_broadcast_task_logs_kind ON broadcast_task_logs (
  kind
);

CREATE TABLE IF NOT EXISTS broadcast_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  broadcast_task_log_id INTEGER NOT NULL REFERENCES broadcast_task_logs(id),
  chat_id INTEGER NOT NULL,
  "group" TEXT NOT NULL DEFAULT '',
  error TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_broadcast_logs_chat_id ON broadcast_logs (
  chat_id
);

CREATE TABLE IF NOT EXISTS recent_teachers(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL REFERENCES chats(id) ON DELETE CASCADE ON UPDATE CASCADE,
	teacher_id TEXT NOT NULL,
	teacher_name TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_recent_teachers_chat_id ON recent_teachers(chat_id);
CREATE INDEX IF NOT EXISTS idx_recent_teachers_created_at ON recent_teachers(created_at);
