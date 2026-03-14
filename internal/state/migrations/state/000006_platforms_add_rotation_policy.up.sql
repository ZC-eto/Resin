ALTER TABLE platforms
ADD COLUMN rotation_policy TEXT NOT NULL DEFAULT 'TTL';

ALTER TABLE platforms
ADD COLUMN rotation_interval_ns INTEGER NOT NULL DEFAULT 0;

UPDATE platforms
SET rotation_policy = CASE
    WHEN sticky_ttl_ns > 0 THEN 'TTL'
    ELSE 'KEEP'
END
WHERE rotation_policy = 'TTL';

UPDATE platforms
SET rotation_interval_ns = sticky_ttl_ns
WHERE rotation_interval_ns = 0 AND sticky_ttl_ns > 0;
