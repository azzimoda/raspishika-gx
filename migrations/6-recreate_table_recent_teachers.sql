DROP TABLE IF EXISTS recent_teachers;
DROP INDEX IF EXISTS idx_recent_teachers_chat_id;
DROP INDEX IF EXISTS idx_recent_teachers_created_at;

CREATE TABLE IF NOT EXISTS recent_teachers(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL REFERENCES chats(id) ON DELETE CASCADE ON UPDATE CASCADE,
	teacher_id TEXT NOT NULL,
	teacher_name TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_recent_teachers_chat_id ON recent_teachers(chat_id);
CREATE INDEX IF NOT EXISTS idx_recent_teachers_created_at ON recent_teachers(created_at);
