// Package config 提供 emotion-echo 各 Go svc 的统一配置加载（Stage 41 PR-0）。
//
// 行为契约见 config_test.go 与 docs/stages/stage-41-gozero-removal.md。
//
// 实现要点：
//   - Load/MustLoad 用 os.ReadFile + yaml.v3
//   - LoadBytes 同 Load 但接收 []byte（供测试）
//   - defaults() 钩子在 Unmarshal 之前执行，调用者负责"零值字段填充默认"
//   - **大小写不敏感**：yaml key 全转小写后再 Unmarshal（A2 假设修正：实测 yaml.v3
//     默认大小写敏感；plan §八 A2 "大小写不敏感" 假设**证伪**，本实现做 key lowercase
//     预处理，确保 6 份 etc/*.yaml 的大写风格（`Name: emotion-echo-chat-svc`）能加载）
//   - 不开 KnownFields（plan §四 4.1：保守起步）
//   - 不解析 ${VAR:-default} 占位（语义不变，由业务侧 applyEnvOverrides 接管）
//
// R3 反向：defaults 顺序写反会导致 yaml 显式 false 被默认 true 覆盖；
//         测试 TestLoad_DefaultsBeforeYaml 已钉死，勿改顺序。
package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load 从 path 读取 yaml，先填 defaults()，再 Unmarshal 到 dst。
//
// 参数：
//   - path: yaml 文件绝对/相对路径
//   - dst: 指向目标 struct 的指针（如 &config.Config{}）
//   - defaults: 无参函数，调用者在此填零值字段的默认值；
//     必须仅设置"零值才填"，不要覆盖 yaml 显式值 —— 这就是 R3 的关键。
//
// 返回 error：文件缺失、读取失败、yaml 语法错。
func Load(path string, dst any, defaults func()) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	return LoadBytes(data, dst, defaults)
}

// MustLoad 同 Load，失败即 panic。供 main 启动期使用（与 go-zero conf.MustLoad 语义一致）。
func MustLoad(path string, dst any, defaults func()) {
	if err := Load(path, dst, defaults); err != nil {
		panic(err)
	}
}

// LoadBytes 同 Load 但接收 []byte（供测试和嵌入式场景使用）。
//
// 顺序（关键，R3 反向风险）：
//   1. defaults() —— 调用者填零值字段
//   2. yaml key lowercase 预处理（A2 假设证伪修正）
//   3. yaml.Unmarshal —— yaml 显式值覆盖默认
//
// 如果调换为"先 yaml 后 defaults"，yaml 显式 false 会被默认 true 覆盖，
// 触发 R3（plan §六 R3）。测试 TestLoad_DefaultsBeforeYaml 已钉死此契约。
func LoadBytes(data []byte, dst any, defaults func()) error {
	if defaults != nil {
		defaults()
	}
	if len(data) == 0 {
		return nil
	}

	// 大小写归一：yaml.v3 默认对 struct 字段名是大小写敏感的，
	// 但 6 份 etc/*.yaml 全是大写风格（`Name:`, `Port:`, `Kafka.BrokersCSV:` 等），
	// 为避免给 145 个 struct 字段加 yaml tag，这里在 Unmarshal 前把所有 key lowercase。
	normalized, err := lowercaseYAMLKeys(data)
	if err != nil {
		return fmt.Errorf("config: normalize yaml keys: %w", err)
	}

	if err := yaml.Unmarshal(normalized, dst); err != nil {
		return fmt.Errorf("config: yaml unmarshal: %w", err)
	}
	return nil
}

// lowercaseYAMLKeys 把 yaml 字节流里所有 map key 转小写，保留值原样。
//
// 实现思路：用 yaml.v3 把 yaml 解到 map[string]any（string-keyed map），递归把
// 所有 key 转为小写，再用 yaml.v3 编码回字节流。
//
// 已知限制：
//   - 不处理 flow style（`{Name: foo}`）—— 现有 6 份 yaml 都用 block style，无此 case
//   - 不处理 yaml alias / anchor —— 现有 6 份 yaml 无此 case
//   - 性能：6 份 etc yaml 几 KB，每次启动解析 2 次（解→编）成本可忽略
func lowercaseYAMLKeys(data []byte) ([]byte, error) {
	var m any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	lowercaseMapKeys(m)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// lowercaseMapKeys 递归把 map 的 string key 全部 lowercase，slice 元素若是 map 同样处理。
func lowercaseMapKeys(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			lk := toLowerASCII(k)
			if lk != k {
				delete(x, k)
				x[lk] = val
				k = lk
			}
			lowercaseMapKeys(x[k])
		}
	case map[any]any:
		for k, val := range x {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			lk := toLowerASCII(ks)
			if lk != ks {
				delete(x, k)
				x[lk] = val
				k = lk
			}
			lowercaseMapKeys(x[k])
		}
	case []any:
		for _, item := range x {
			lowercaseMapKeys(item)
		}
	}
}

// toLowerASCII 仅对 ASCII 大写字母 lowercase（不引入 strings 包依赖，
// 也避免对非 ASCII 字符做 unicode 大小写转换导致行为差异）。
func toLowerASCII(s string) string {
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

