package main

import (
	"reflect"
	"testing"
)

// Stage 26-P · Commit P3 单测:splitBrokersCSV() 是 main.go 的 helper,
// 验证它能正确把 `${KAFKA_BROKERS:-host:9092}` 解析的 csv 字符串切分
// 成 events.NewKafkaEventPublisher 期望的 []string。
func TestSplitBrokersCSV(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single broker", "emotion-echo-kafka:9092", []string{"emotion-echo-kafka:9092"}},
		{"two brokers comma", "k1:9092,k2:9092", []string{"k1:9092", "k2:9092"}},
		{"trims whitespace", " k1:9092 , k2:9092 ", []string{"k1:9092", "k2:9092"}},
		{"empty string", "", nil},
		{"only commas", ",,,", []string{}},
		{"trailing comma", "kafka:9092,", []string{"kafka:9092"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitBrokersCSV(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitBrokersCSV(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
