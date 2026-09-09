-- +goose Up
CREATE TABLE schedule_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    group_name TEXT NOT NULL,
    department_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (chat_id, group_name)
);

-- Переносим выбранные группы, сохраняя время и состояние рассылки в chats.
INSERT INTO schedule_subscriptions (chat_id, group_name, department_name)
SELECT id, "group", COALESCE(department, '') FROM chats
WHERE "group" IS NOT NULL AND "group" != '';

-- +goose Down
DROP TABLE schedule_subscriptions;
