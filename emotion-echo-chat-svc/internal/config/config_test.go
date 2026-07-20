package config

import "testing"

// TestConfig_DefaultValues 静态断言 Config 各字段默认值（来自 struct tag）
func TestConfig_DefaultValues(t *testing.T) {
	c := Config{}
	if c.Name != "" {
		t.Fatalf("zero Name")
	}
	if c.Port != 0 {
		t.Fatalf("zero Port")
	}
	if c.SkyWalking.OAPAddr != "" {
		t.Fatalf("zero OAPAddr")
	}
	if c.Postgres.MaxOpenConns != 0 {
		t.Fatalf("zero MaxOpenConns")
	}
	if c.Kafka.GroupID != "" {
		t.Fatalf("zero GroupID")
	}
}

// TestSkyWalking_Struct 字段读写
func TestSkyWalking_Struct(t *testing.T) {
	sw := SkyWalking{
		OAPAddr:     "localhost:11800",
		ServiceName: "emotion-echo-chat-svc",
		Enabled:     true,
	}
	if sw.OAPAddr != "localhost:11800" || sw.ServiceName != "emotion-echo-chat-svc" {
		t.Fatalf("field mismatch: %+v", sw)
	}
	if !sw.Enabled {
		t.Fatalf("should be enabled")
	}
}

// TestPostgres_Struct 表驱动
func TestPostgres_Struct(t *testing.T) {
	cases := []struct {
		dsn         string
		maxOpen     int
		maxIdle     int
		wantOpen    int
		wantIdle    int
	}{
		{"host=db", 10, 5, 10, 5},
		{"host=x user=u", 20, 10, 20, 10},
		{"", 0, 0, 0, 0},
	}
	for _, tc := range cases {
		p := Postgres{
			DSN:          tc.dsn,
			MaxOpenConns: tc.maxOpen,
			MaxIdleConns: tc.maxIdle,
		}
		if p.MaxOpenConns != tc.wantOpen || p.MaxIdleConns != tc.wantIdle {
			t.Fatalf("postgres mismatch: want open=%d idle=%d, got %+v", tc.wantOpen, tc.wantIdle, p)
		}
	}
}

// TestKafka_Struct 字段读写 — Stage 26-P 改造后 Brokers list 改 BrokersCSV string。
func TestKafka_Struct(t *testing.T) {
	k := Kafka{
		BrokersCSV: "k1:9092,k2:9092",
		GroupID:    "chat-svc",
		Enabled:    true,
	}
	if k.BrokersCSV != "k1:9092,k2:9092" {
		t.Fatalf("brokers csv mismatch: %q", k.BrokersCSV)
	}
	if k.GroupID != "chat-svc" {
		t.Fatalf("group id mismatch")
	}
}
