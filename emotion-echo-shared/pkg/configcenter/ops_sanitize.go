// Package configcenter — ops 配置内容层清洗（E2E-23 P1，账本 E2E-F-158）
//
// 为什么需要这一层（既有保护为什么不够）：
//
//	既有 `sensitivePrefixes` 只在 `isSensitiveDataId` 里生效，而后者**仅被
//	PublishConfig 调用**。也就是说它保护的是"**dataId**"这一层。
//	而 E2E-23 D-32 决议的 ops 方案是"**单 dataId 打包全部运营参数**"
//	（`emotion-echo-<svc>.ops.yaml`）—— dataId 本身不匹配任何敏感前缀，
//	于是往该文件里写 `llm.api_key: sk-xxx` 可以畅通无阻。
//
//	两个保护管的是两件事，缺一不可：
//	  - dataId 层：防止把**整个**敏感配置推到 Nacos（既有实现）
//	  - 内容层：防止在**合法 dataId** 里夹带敏感 key（本文）
//
// 用法：ops 配置在反序列化**之前**先过一遍 SanitizeOpsContent。
//
//	cleaned, dropped := SanitizeOpsContent(raw)
//	// dropped 非空即说明有人往 ops 里塞了敏感项 —— 调用方应打 WARN
//
// 设计取舍：按行剔除而非 YAML 解析后删除，理由有二 ——
//  1. 未知 key 本来就会被固定 struct 忽略，剔除与否都不影响行为；
//     但**值**会残留在内存/日志里，剔除更干净。
//  2. 不引入 yaml 解析依赖，且能保留注释与格式，运维可读性不受损。
package configcenter

import "strings"

// sensitiveBaseNames 是敏感词的"词根"形态。
//
// 为什么单独维护：既有列表里的 `postgres_password` / `.token` 等是按
// **dataId 字符串**形态写的（点号分隔），而 ops 里的 key 大量使用
// **下划线/短横线**（`auth_token` / `main_dsn` / `xtts-timeout`）。
// 若直接拿 dataId 规则去匹配 key，`auth_token` 会被切成
// ["auth","token"] 而 `.token` 后缀匹配不到整串 ⇒ **漏判**。
// 词根集合让两种命名风格都能被覆盖。
// credentialNames 是**凭据类**词根：出现在 key 的任意段即判敏感，
// 与分隔符无关（`auth_token` / `db.password` / `main-dsn` 都命中）。
var credentialNames = map[string]bool{
	"password": true,
	"secret":   true,
	"token":    true,
	"dsn":      true,
	"jwt":      true,
	"apikey":   true,
	"api_key":  true,
}

// componentNames 是**组件/服务类**词根：仅当它们作为点号命名空间的
// 第一段时判敏感（`llm.api_key`），而 `llm_timeout` 这类**运营参数名**
// 不判 —— 后者被误删会导致合法配置静默失效（比漏判更难排查）。
var componentNames = map[string]bool{
	"llm":      true,
	"kafka":    true,
	"db":       true,
	"database": true,
	"openai":   true,
	"deepseek": true,
	"postgres": true,
}

// IsSensitiveOpsKey 判定 ops 配置里的一个 key 是否敏感。
//
// 判定规则：
//   - 按 `.` `_` `-` 切段，**任一段**命中敏感词根即判敏感。
//     （`llm.api_key` → llm 命中；`auth_token` → token 命中；
//     `db.password` → db 命中；`main_dsn` → dsn 命中）
//   - 整串命中 sensitiveSuffixes 亦判敏感（覆盖 `.token` 这类带点写法）
//   - 大小写不敏感
//
// 刻意**不做**前缀通配：`limit_count` / `xtts_timeout_s` 这类正常运营
// 参数不能被误判 —— 误判会让合法配置静默失效，比漏判更难排查。
func IsSensitiveOpsKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	// 整串后缀（覆盖 db.token / main.dsn 这类点号写法）
	for _, s := range sensitiveSuffixes {
		if strings.HasSuffix(k, s) {
			return true
		}
	}
	// 整串前缀：**仅保留带点号的形态**（`llm.xxx` / `db.xxx`）。
	//
	// 不能退化成 `HasPrefix(k, "llm")` —— 那样 `llm_timeout` 会被误判
	// （E2E-23 实施期实测踩到：整份 ai-svc ops 配置被剔得只剩 7 项）。
	// 无分隔符的老前缀（如 `postgres_password`）由 credentialNames 的
	// `password` 段覆盖，不需要在这里兜。
	for _, p := range sensitivePrefixes {
		if strings.HasSuffix(p, ".") && strings.HasPrefix(k, p) {
			return true
		}
	}
	// 逐段词根。
	//
	// ⚠️ 关键区分（E2E-23 实施期实测踩到）：`llm` / `kafka` / `db` 这类
	// **服务名或组件名**出现在 key 里时，往往是**合法运营参数**
	// （`llm_timeout` / `kafka_max_retries` / `db_pool_size`），
	// 而非敏感凭据。真正敏感的是它们作为**命名空间前缀**时的子项
	// （`llm.api_key` / `db.password`）。
	//
	// 故：组件类词根**仅在点号命名空间下**判定（`llm.xxx`），
	// 下划线形式（`llm_timeout`）不判敏感；而凭据类词根
	// （password / secret / token / dsn）无论何种分隔符都判敏感。
	segs := strings.FieldsFunc(k, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	for _, seg := range segs {
		if credentialNames[seg] {
			return true
		}
	}
	// 点号命名空间的组件前缀：`llm.api_key` / `db.password`
	if i := strings.Index(k, "."); i > 0 {
		if componentNames[k[:i]] {
			return true
		}
	}
	return false
}

// SanitizeOpsContent 剔除 ops 配置内容里的敏感 key 及其值。
//
// 返回清洗后的内容与被剔除的 key 列表（嵌套时按点号路径上报，如
// `db.password`，便于运维在日志里定位是哪条配置越界）。
// 只处理 YAML 的 `key: value` 形式；列表项与多行块标量不解析
// （不在 ops 运营参数的用法范围内）。
func SanitizeOpsContent(content string) (string, []string) {
	if content == "" {
		return content, nil
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	var dropped []string
	// droppedStack 记录"被剔除的父 key"，用于拼出点号路径
	var droppedStack []string
	dropValueIndent := -1 // ≥0 表示"正在丢弃该缩进层及更深的所有行"

	for _, line := range lines {
		indent := indentOf(line)
		trimmed := strings.TrimSpace(line)

		// 处于"丢弃值"状态：缩进更深 ⇒ 仍属该 key 的值，继续丢
		if dropValueIndent >= 0 {
			if trimmed == "" || indent > dropValueIndent {
				// 嵌套的敏感子项也要按点号路径上报（db: / password: ⇒ db.password），
				// 否则日志只会说 "db" 被丢，运维定位不到具体越界项。
				if trimmed != "" && indent > dropValueIndent {
					if idx := strings.Index(trimmed, ":"); idx > 0 {
						child := strings.TrimSpace(trimmed[:idx])
						if child != "" && !strings.ContainsAny(child, "#") && IsSensitiveOpsKey(child) {
							dropped = append(dropped,
								strings.Join(append(append([]string{}, droppedStack...), child), "."))
						}
					}
				}
				continue
			}
			dropValueIndent = -1
			for len(droppedStack) > 0 {
				droppedStack = droppedStack[:len(droppedStack)-1]
			}
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" {
			out = append(out, line)
			continue
		}

		idx := strings.Index(trimmed, ":")
		if idx <= 0 {
			out = append(out, line)
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		if key == "" || strings.ContainsAny(key, "#") {
			out = append(out, line)
			continue
		}

		if IsSensitiveOpsKey(key) {
			dropped = append(dropped, strings.Join(append(append([]string{}, droppedStack...), key), "."))
			if strings.TrimSpace(trimmed[idx+1:]) == "" {
				// 值在后续行 → 记录缩进层级，并把本 key 作为父级
				dropValueIndent = indent
				droppedStack = append(droppedStack, key)
			}
			continue // 不写入 out
		}

		out = append(out, line)
	}

	if len(dropped) == 0 {
		return content, nil // 无改动时原样返回
	}
	return strings.Join(out, "\n"), dropped
}

func indentOf(line string) int {
	n := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			break
		}
		n++
	}
	return n
}
