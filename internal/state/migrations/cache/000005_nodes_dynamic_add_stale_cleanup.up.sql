ALTER TABLE nodes_dynamic
ADD COLUMN stale_cleanup_window_started_at_ns INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN stale_cleanup_last_observed_probe_at_ns INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN stale_cleanup_failed_probe_count INTEGER NOT NULL DEFAULT 0;
