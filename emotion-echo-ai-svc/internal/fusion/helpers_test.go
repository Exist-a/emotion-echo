// Package fusion — test helpers
//
// 测试用 JSON 解析帮手（与 production code 共享语义但不依赖外部包）。
package fusion

import "encoding/json"

// jsonUnmarshalFloat 从 JSON 字符串中取出指定 key 的 float 值。
func jsonUnmarshalFloat(s, key string, dst *float64) error {
	m := map[string]float64{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return err
	}
	*dst = m[key]
	return nil
}

// jsonUnmarshalMap 把 JSON 字符串解析为 map。
func jsonUnmarshalMap(s string, dst *map[string]float64) error {
	return json.Unmarshal([]byte(s), dst)
}
