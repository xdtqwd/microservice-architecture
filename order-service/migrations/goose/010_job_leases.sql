-- +goose Up
-- Расписание фоновых задач, которые должны выполняться один раз на весь
-- сервис, а не в каждой реплике. Реплика на тике пытается сдвинуть
-- next_run_at вперёд условным UPDATE — удаётся ровно одной.
CREATE TABLE job_leases (
    name        TEXT PRIMARY KEY,
    next_run_at TIMESTAMPTZ NOT NULL
);
INSERT INTO job_leases (name, next_run_at) VALUES ('reconciliation', now());

-- +goose Down
DROP TABLE job_leases;
