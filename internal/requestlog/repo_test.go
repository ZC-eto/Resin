package requestlog

import (
	"path/filepath"
	"testing"

	"github.com/Resinat/Resin/internal/state"
)

func TestRepoOpen_UpgradesLegacyRequestLogColumns(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "request_logs-1.db")

	db, err := state.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE request_logs (
	id TEXT PRIMARY KEY,
	ts_ns INTEGER NOT NULL,
	proxy_type INTEGER NOT NULL,
	client_ip TEXT NOT NULL DEFAULT '',
	platform_id TEXT NOT NULL DEFAULT '',
	platform_name TEXT NOT NULL DEFAULT '',
	account TEXT NOT NULL DEFAULT '',
	target_host TEXT NOT NULL DEFAULT '',
	target_url TEXT NOT NULL DEFAULT '',
	node_hash TEXT NOT NULL DEFAULT '',
	node_tag TEXT NOT NULL DEFAULT '',
	egress_ip TEXT NOT NULL DEFAULT '',
	duration_ns INTEGER NOT NULL DEFAULT 0,
	net_ok INTEGER NOT NULL DEFAULT 0,
	http_method TEXT NOT NULL DEFAULT '',
	http_status INTEGER NOT NULL DEFAULT 0,
	resin_error TEXT NOT NULL DEFAULT '',
	upstream_stage TEXT NOT NULL DEFAULT '',
	upstream_err_kind TEXT NOT NULL DEFAULT '',
	upstream_errno TEXT NOT NULL DEFAULT '',
	upstream_err_msg TEXT NOT NULL DEFAULT '',
	ingress_bytes INTEGER NOT NULL DEFAULT 0,
	egress_bytes INTEGER NOT NULL DEFAULT 0,
	payload_present INTEGER NOT NULL DEFAULT 0,
	req_headers_len INTEGER NOT NULL DEFAULT 0,
	req_body_len INTEGER NOT NULL DEFAULT 0,
	resp_headers_len INTEGER NOT NULL DEFAULT 0,
	resp_body_len INTEGER NOT NULL DEFAULT 0,
	req_headers_truncated INTEGER NOT NULL DEFAULT 0,
	req_body_truncated INTEGER NOT NULL DEFAULT 0,
	resp_headers_truncated INTEGER NOT NULL DEFAULT 0,
	resp_body_truncated INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE request_log_payloads (
	log_id TEXT PRIMARY KEY,
	req_headers BLOB,
	req_body BLOB,
	resp_headers BLOB,
	resp_body BLOB
);
`)
	if err != nil {
		t.Fatalf("seed legacy request log schema: %v", err)
	}
	_ = db.Close()

	repo := NewRepo(dir, 64*1024*1024, 2)
	if err := repo.Open(); err != nil {
		t.Fatalf("Repo.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	checkDB, err := state.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB upgraded: %v", err)
	}
	defer checkDB.Close()

	for _, column := range []string{
		"access_mode",
		"lease_action",
		"rotate_requested",
		"rotate_applied",
		"rotate_source",
		"previous_egress_ip",
	} {
		ok, err := hasSQLiteColumn(checkDB, "request_logs", column)
		if err != nil {
			t.Fatalf("hasSQLiteColumn(%s): %v", column, err)
		}
		if !ok {
			t.Fatalf("missing migrated column %q", column)
		}
	}
}
