package config

import "testing"

// TestConfig_DefaultValues 静态断言
func TestConfig_DefaultValues(t *testing.T) {
	c := Config{}
	if c.Name != "" || c.Host != "" || c.Port != 0 {
		t.Fatalf("zero Config should be all zero, got %+v", c)
	}
}

// TestSkyWalking_Struct 字段读写
func TestSkyWalking_Struct(t *testing.T) {
	sw := SkyWalking{OAPAddr: "oap:11800", ServiceName: "svc", Enabled: true}
	if !sw.Enabled {
		t.Fatalf("Enabled not set")
	}
}

// TestPostgres_Struct 表驱动
func TestPostgres_Struct(t *testing.T) {
	p := Postgres{DSN: "host=db user=u", MaxOpenConns: 20, MaxIdleConns: 10}
	if p.MaxOpenConns != 20 || p.MaxIdleConns != 10 {
		t.Fatalf("field mismatch: %+v", p)
	}
}
