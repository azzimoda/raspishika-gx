-- +goose NO TRANSACTION
-- +goose Up
-- Мультиплатформенная поддержка: добавляем дискриминатор платформы в chats,
-- чтобы VK peer id не мог совпасть с Telegram chat id в общей БД
-- (старое UNIQUE(tg_chat_id) это не гарантировало). SQLite не умеет менять
-- UNIQUE-ограничение, поэтому таблица пересоздаётся; данные и id сохраняются.

PRAGMA foreign_keys=OFF;

CREATE TABLE chats_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	tg_chat_id INTEGER NOT NULL,
	platform TEXT NOT NULL DEFAULT 'telegram',
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
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (platform, tg_chat_id)
);

INSERT INTO chats_new (
	id, tg_chat_id, username, state, department, "group", daily_sending_time,
	pair_sending, update_notification, dark_mode, access, created_at, updated_at)
SELECT id, tg_chat_id, username, state, department, "group", daily_sending_time,
	pair_sending, update_notification, dark_mode, access, created_at, updated_at
FROM chats;

DROP TABLE chats;
ALTER TABLE chats_new RENAME TO chats;

CREATE INDEX idx_chats_tg_chat_id ON chats(tg_chat_id);
CREATE INDEX idx_chats_group ON chats("group");
CREATE INDEX idx_chats_daily_sending_time ON chats(daily_sending_time);

PRAGMA foreign_keys=ON;

-- +goose Down
-- Восстановление старой схемы (обычно не требуется, приведено для симметрии).
PRAGMA foreign_keys=OFF;

CREATE TABLE chats_old (
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

INSERT INTO chats_old (
	id, tg_chat_id, username, state, department, "group", daily_sending_time,
	pair_sending, update_notification, dark_mode, access, created_at, updated_at)
SELECT id, tg_chat_id, username, state, department, "group", daily_sending_time,
	pair_sending, update_notification, dark_mode, access, created_at, updated_at
FROM chats;

DROP TABLE chats;
ALTER TABLE chats_old RENAME TO chats;
CREATE INDEX idx_chats_tg_chat_id ON chats(tg_chat_id);
CREATE INDEX idx_chats_group ON chats("group");
CREATE INDEX idx_chats_daily_sending_time ON chats(daily_sending_time);

PRAGMA foreign_keys=ON;