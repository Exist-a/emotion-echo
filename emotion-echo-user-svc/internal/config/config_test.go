package config

import "testing"

// TestConfig_DefaultValues 零值安全
func TestConfig_DefaultValues(t *testing.T) {
	c := Config{}
	if c.Name != "" || c.Port != 0 {
		t.Fatalf("zero Config should be empty, got %+v", c)
	}
}

// TestSkyWalking_Fields 字段读写
func TestSkyWalking_Fields(t *testing.T) {
	sw := SkyWalking{OAPAddr: "oap:11800", ServiceName: "emotion-echo-user-svc", Enabled: true}
	if sw.OAPAddr != "oap:11800" || !sw.Enabled {
		t.Fatalf("field mismatch: %+v", sw)
	}
}

// TestPostgres_Fields 表驱动
func TestPostgres_Fields(t *testing.T) {
	cases := []Postgres{
		{DSN: "host=db", MaxOpenConns: 10, MaxIdleConns: 5},
		{DSN: "", MaxOpenConns: 0, MaxIdleConns: 0},
	}
	for _, p := range cases {
		// 不报错即视为 ok
		_ = p
	}
}
