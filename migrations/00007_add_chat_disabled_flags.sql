-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
ALTER TABLE chats ADD COLUMN daily_disabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE chats ADD COLUMN pair_disabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE chats ADD COLUMN change_disabled BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE chats DROP COLUMN daily_disabled;
ALTER TABLE chats DROP COLUMN pair_disabled;
ALTER TABLE chats DROP COLUMN change_disabled;
