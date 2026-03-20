CREATE TABLE IF NOT EXISTS egress_profile_cache (
    egress_ip TEXT PRIMARY KEY,
    egress_network_type TEXT NOT NULL DEFAULT 'UNKNOWN',
    egress_asn INTEGER NOT NULL DEFAULT 0,
    egress_asn_name TEXT NOT NULL DEFAULT '',
    egress_asn_type TEXT NOT NULL DEFAULT '',
    egress_provider TEXT NOT NULL DEFAULT '',
    egress_profile_source TEXT NOT NULL DEFAULT '',
    egress_profile_updated_at_ns INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_egress_profile_cache_updated_at
    ON egress_profile_cache (egress_profile_updated_at_ns);
