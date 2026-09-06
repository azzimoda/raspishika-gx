-- +goose Up
-- Ручные массовые рассылки админ-бота (cmd/adminbot): по одной строке на
-- платформу, воркер каждого процесса забирает только свои platform-строки.

CREATE TABLE IF NOT EXISTS broadcast_jobs(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	audience TEXT NOT NULL, -- all | private | groups | active | by_group | by_department
	platform TEXT NOT NULL, -- telegram | vk
	spec TEXT, -- параметры аудитории (группа, отделение, период) как JSON
	html TEXT NOT NULL, -- тело сообщения (HTML)
	status TEXT NOT NULL DEFAULT 'pending', -- pending | sending | done | failed
	created_by INTEGER NOT NULL, -- id админ-чата
	claimed_by TEXT, -- экземпляр процесса (host:pid), забравший задачу
	error TEXT,
	finished_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_broadcast_jobs_platform_status ON broadcast_jobs(platform, status, created_at);

-- +goose Down
DROP TABLE IF EXISTS broadcast_jobs;