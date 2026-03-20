ALTER TABLE nodes_dynamic
ADD COLUMN egress_network_type TEXT NOT NULL DEFAULT '';

ALTER TABLE nodes_dynamic
ADD COLUMN egress_asn INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN egress_asn_name TEXT NOT NULL DEFAULT '';

ALTER TABLE nodes_dynamic
ADD COLUMN egress_asn_type TEXT NOT NULL DEFAULT '';

ALTER TABLE nodes_dynamic
ADD COLUMN egress_provider TEXT NOT NULL DEFAULT '';

ALTER TABLE nodes_dynamic
ADD COLUMN egress_profile_source TEXT NOT NULL DEFAULT '';

ALTER TABLE nodes_dynamic
ADD COLUMN egress_profile_updated_at_ns INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN quality_score INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN quality_grade TEXT NOT NULL DEFAULT 'D';

ALTER TABLE nodes_dynamic
ADD COLUMN egress_probe_success_count_total INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN egress_probe_failure_count_total INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN egress_ip_change_count_total INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN last_egress_ip_change_at_ns INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN circuit_open_count_total INTEGER NOT NULL DEFAULT 0;
