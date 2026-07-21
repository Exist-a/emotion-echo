package config

import "testing"

// TestConfig_DefaultValues 表驱动：Config 各字段零值
func TestConfig_DefaultValues(t *testing.T) {
	c := Config{}
	if c.Name != "" || c.Port != 0 || c.Host != "" {
		t.Fatalf("zero Config should be all zero, got %+v", c)
	}
}

// TestSkyWalking_Struct 字段读写
func TestSkyWalking_Struct(t *testing.T) {
	sw := SkyWalking{
		OAPAddr:     "oap:11800",
		ServiceName: "emotion-echo-analytics-svc",
		Enabled:     true,
	}
	if !sw.Enabled || sw.OAPAddr != "oap:11800" {
		t.Fatalf("field mismatch: %+v", sw)
	}
}

// TestPostgres_Struct 表驱动 max conns
func TestPostgres_Struct(t *testing.T) {
	cases := []struct {
		maxOpen, maxIdle int
	}{
		{10, 5},
		{50, 25},
		{0, 0},
	}
	for _, tc := range cases {
		p := Postgres{
			DSN:          "host=db user=u",
			MaxOpenConns: tc.maxOpen,
			MaxIdleConns: tc.maxIdle,
		}
		if p.MaxOpenConns != tc.maxOpen || p.MaxIdleConns != tc.maxIdle {
			t.Fatalf("postgres mismatch: want open=%d idle=%d, got %+v", tc.maxOpen, tc.maxIdle, p)
		}
	}
}
