package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

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

// TestKafka_DefaultsViaYamlLoad 表驱动：Kafka 配置默认值（与 ai-svc 同模式）。
//
// go-zero conf 的 `json:",default=..."` 只在 yaml 加载时生效，零值 struct
// 不应用 default tag → 用 conf.MustLoad 验证。注意：go-zero 的 slice
// default tag 会保留字面引号（`["chat-events"]`），故 Topics/BrokersCSV
// 在 yaml 显式给出；GroupID / Enabled 依赖 default tag。
func TestKafka_DefaultsViaYamlLoad(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "kafka-test.yaml")
	body := `Name: kafka-test
Host: 0.0.0.0
Port: 8893
SkyWalking:
  OAPAddr: localhost:11800
  ServiceName: test
  Enabled: false
Postgres:
  DSN: "host=localhost dbname=x"
Kafka:
  BrokersCSV: "kafka1:9092,kafka2:9092"
  Topics: ["chat-events"]
`
	require.NoError(t, os.WriteFile(yamlPath, []byte(body), 0o644))

	var c Config
	conf.MustLoad(yamlPath, &c)

	if c.Kafka.BrokersCSV != "kafka1:9092,kafka2:9092" {
		t.Fatalf("BrokersCSV mismatch, got %q", c.Kafka.BrokersCSV)
	}
	if c.Kafka.GroupID != "analytics-svc" {
		t.Fatalf("GroupID default mismatch, got %q", c.Kafka.GroupID)
	}
	if c.Kafka.Enabled {
		t.Fatal("Enabled should default false (opt-in)")
	}
	if len(c.Kafka.Topics) != 1 || c.Kafka.Topics[0] != "chat-events" {
		t.Fatalf("Topics mismatch, got %v", c.Kafka.Topics)
	}
}

// TestKafka_Struct 字段读写
func TestKafka_Struct(t *testing.T) {
	k := Kafka{
		BrokersCSV: "kafka1:9092,kafka2:9092",
		GroupID:    "analytics-test",
		Enabled:    true,
		Topics:     []string{"chat-events", "other"},
	}
	if k.BrokersCSV != "kafka1:9092,kafka2:9092" || k.GroupID != "analytics-test" ||
		!k.Enabled || len(k.Topics) != 2 {
		t.Fatalf("field mismatch: %+v", k)
	}
}
