-- +goose Up
-- Drop the legacy hand-rolled migration tracking table (pre-goose).
DROP TABLE IF EXISTS migrations;
