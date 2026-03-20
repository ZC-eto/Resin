ALTER TABLE platforms
ADD COLUMN subscription_filters_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE platforms
ADD COLUMN network_type_filters_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE platforms
ADD COLUMN min_quality_score INTEGER;

ALTER TABLE platforms
ADD COLUMN max_reference_latency_ms INTEGER;

ALTER TABLE platforms
ADD COLUMN min_egress_stability_score INTEGER;

ALTER TABLE platforms
ADD COLUMN max_circuit_open_count INTEGER;
