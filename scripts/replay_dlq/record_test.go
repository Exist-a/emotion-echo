package main

import (
	"strings"
	"testing"
)

// TestPgDSNFromEnv_Defaults dev 缺省口径（与 migrate.sh / backup_db.sh 一致）。
func TestPgDSNFromEnv_Defaults(t *testing.T) {
	got := pgDSNFromEnv(func(string) string { return "" })
	want := "postgres://postgres:postgres@localhost:5432/emotion_echo?sslmode=disable"
	if got != want {
		t.Fatalf("缺省 DSN 不符\n got=%s\nwant=%s", got, want)
	}
}

// TestPgDSNFromEnv_Overrides 环境变量逐项覆盖（容器内 PGHOST=postgres 的场景）。
func TestPgDSNFromEnv_Overrides(t *testing.T) {
	env := map[string]string{
		"PGUSER": "analytics_reader", "PGPASSWORD": "s3cr3t",
		"PGHOST": "postgres", "PGPORT": "5433", "PGDATABASE": "emotion_echo",
	}
	got := pgDSNFromEnv(func(k string) string { return env[k] })
	for _, frag := range []string{"analytics_reader:s3cr3t@", "@postgres:5433/", "/emotion_echo?", "sslmode=disable"} {
		if !strings.Contains(got, frag) {
			t.Errorf("DSN 缺片段 %q: %s", frag, got)
		}
	}
}
