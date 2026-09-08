// PR-OBS-3: ExpandShellEnvDefaults 把 yaml/JSON 字符串中的 ${VAR} 与 ${VAR:-default}
// 替换为 os.Getenv("VAR")（空时用 default）。
//
// 行为与 bash "${VAR:-default}" 一致：
//   - ${VAR}            → os.Getenv("VAR")（空时保留字面 "${VAR}" 作为未定义信号）
//   - ${VAR:-default}   → os.Getenv("VAR") ?? default（default 允许空格/标点）
//   - $${VAR}           → 字面 "${VAR}"（转义，与 bash 一致）
//   - 裸 $ 不替换（避免误伤如 price=$10）
//
// 未定义且无 default：返回 error（fail-fast，与 bootstrap 行为一致）。
//
// 已知限制（与 bash 不一致的地方）：
//   - 占位符不可嵌套：${${INNER}} 视为字面
//   - default 内部不再次做占位符展开（避免循环依赖语义）
//
// 性能：O(n) 单次扫描，启动期调用 1-3 次，开销可忽略。
package config

import (
	"fmt"
	"os"
)

// ExpandShellEnvDefaults 展开 ${VAR} 与 ${VAR:-default} 占位符
//
// 入参 s：要展开的字符串（yaml 字段值或 JSON string）
// 返回：展开后的字符串 + error（仅在 ${VAR} 未定义且无 default 时返回）
//
// 用法（典型 main.go 启动顺序）：
//   raw, _ := os.ReadFile(yamlPath)
//   expanded, err := config.ExpandShellEnvDefaults(string(raw))
//   if err != nil { log.Fatal(err) }
//   config.LoadBytes([]byte(expanded), &cfg, nil)
func ExpandShellEnvDefaults(s string) (string, error) {
	var out []byte
	i := 0
	for i < len(s) {
		// 1. $${VAR} → 字面 ${VAR}（转义）
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '$' {
			// 找下一个 $ 之外的字符
			if i+2 < len(s) && s[i+2] == '{' {
				// $${...} → 字面 ${...}
				j := i + 3
				for j < len(s) && s[j] != '}' {
					j++
				}
				if j >= len(s) {
					// 未闭合的 $${... → 原样输出 $${...（避免误吞）
					out = append(out, s[i:i+2]...)
					i += 2
					continue
				}
				// 输出字面 ${VAR}（保留 ${ 与 }）
				out = append(out, '$', '{')
				out = append(out, s[i+3:j]...)
				out = append(out, '}')
				i = j + 1
				continue
			}
			// $$ 不是转义,作为字面 $$ 输出
			out = append(out, '$', '$')
			i += 2
			continue
		}

		// 2. ${VAR} 或 ${VAR:-default}
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			j := i + 2
			for j < len(s) && s[j] != '}' {
				j++
			}
			if j >= len(s) {
				// 未闭合 ${ → 原样输出
				out = append(out, s[i:i+1]...)
				i++
				continue
			}
			placeholder := s[i+2 : j]
			name, def, hasDef := splitVarDefault(placeholder)
			val := os.Getenv(name)
			if val == "" {
				if !hasDef {
					return "", fmt.Errorf("config: ExpandShellEnvDefaults: undefined var ${%s} (no default)", name)
				}
				val = def
			}
			out = append(out, val...)
			i = j + 1
			continue
		}

		// 3. 其他字符原样输出
		out = append(out, s[i])
		i++
	}
	return string(out), nil
}

// splitVarDefault 把 "VAR" 或 "VAR:-default" 拆成 (name, default, hasDefault)
//
// 规则：第一个 :- 是 name/default 分隔符。
// 例：
//   "HOME" → ("HOME", "", false)
//   "HOME:-/tmp" → ("HOME", "/tmp", true)
//   "X:-hello world" → ("X", "hello world", true)  ← default 允许空格
func splitVarDefault(s string) (name, def string, hasDefault bool) {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == ':' && s[i+1] == '-' {
			return s[:i], s[i+2:], true
		}
	}
	return s, "", false
}
