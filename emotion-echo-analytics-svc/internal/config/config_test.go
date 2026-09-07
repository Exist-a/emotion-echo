package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	sharedconfig "github.com/emotion-echo/shared/pkg/config"
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

// TestKafka_DefaultsViaYamlLoad 表驱动：Kafka 配置默认值（Stage 41 PR-6）。
//
// Stage 41 改用 shared/pkg/config 后语义保持:yaml 显式给值 + SetDefaults 填零值。
// 不再依赖 go-zero conf 的 default tag。
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
  Enabled: true
`
	require.NoError(t, os.WriteFile(yamlPath, []byte(body), 0o644))

	var c Config
	sharedconfig.MustLoad(yamlPath, &c, func() { SetDefaults(&c) })

	if c.Kafka.BrokersCSV != "kafka1:9092,kafka2:9092" {
		t.Fatalf("BrokersCSV mismatch, got %q", c.Kafka.BrokersCSV)
	}
	if c.Kafka.GroupID != "analytics-svc" {
		t.Fatalf("GroupID default mismatch, got %q", c.Kafka.GroupID)
	}
	// Stage 41 PR-6:yaml 显式给 Enabled: true(R3 反向测试已钉死 SetDefaults 不覆盖 bool)
	if !c.Kafka.Enabled {
		t.Fatal("Enabled should be true (yaml 显式给值)")
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
