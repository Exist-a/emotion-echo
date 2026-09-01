// Package fusion — Stage 35 · PR-1 GREEN
//
// unwrapLLMContent 把 LLM chat completions 返回的 message.content
// 剥成"干净的 JSON 字符串"供 json.Unmarshal 使用。
//
// 真实 LLM 返回模式（按出现频率）：
//   1. 纯 JSON 字符串（理想情况）
//   2. ```json\n{...}\n``` markdown 包裹（DeepSeek / OpenAI 偶发）
//   3. ```\n{...}\n``` 无语言标记（Llama 类兼容实现）
//   4. 双重 JSON 编码（content 本身是 JSON 字符串，需再反序列化一次）
//   5. 前置/后置自然语言（"以下是融合结果：..."）
//
// 实现策略（按优先级）：
//   1. TrimSpace
//   2. 检测并剥 markdown 三反引号（含可选 json 语言标记）
//   3. 再 TrimSpace
//   4. 若首字符是 '"' 且 Unmarshal 后是 string → 视为双重 JSON，重 Marshal
//   5. 返回最终字符串
package fusion

import (
	"encoding/json"
	"strings"
)

// markdownFenceStart 三反引号开始标记（含可选语言标记）。
const markdownFenceStart = "```"

// markdownFenceEnd 三反引号结束标记。
const markdownFenceEnd = "```"

// markdownFenceStartJSON 带 json 语言标记的开始标记（大小写不敏感在 prefix 上）。
var markdownFenceStartJSONPrefix = markdownFenceStart + "json"

// unwrapLLMContent 把 LLM content 剥成干净 JSON 字符串。
//
// 入参：LLM 返回的 message.content 原始字符串。
// 返回：可直接 json.Unmarshal 的字符串；解析失败保持原 trim 后的字符串。
func unwrapLLMContent(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// 1. markdown 三反引号包裹（含或不含 json 语言标记）
	if strings.HasPrefix(s, markdownFenceStart) && strings.HasSuffix(s, markdownFenceEnd) {
		// 剥掉首尾三反引号
		inner := s[len(markdownFenceStart) : len(s)-len(markdownFenceEnd)]
		// 若首行是语言标记（如 "json" 或 "JSON"）→ 剥掉
		if idx := strings.IndexAny(inner, "\n"); idx > 0 {
			firstLine := strings.TrimSpace(inner[:idx])
			if isFenceLangTag(firstLine) {
				inner = inner[idx+1:]
			}
		}
		s = strings.TrimSpace(inner)
	}

	// 2. 双重 JSON 编码：content 本身是 JSON 字符串
	//    用 json.Unmarshal 试一次，若能解析为 string 类型，视为双重编码
	if len(s) > 0 && s[0] == '"' {
		var unquoted string
		if err := json.Unmarshal([]byte(s), &unquoted); err == nil {
			s = unquoted
		}
	}

	return s
}

// isFenceLangTag 判定字符串是否是常见 markdown 代码块语言标记。
//
// 已知合法：json / JSON / Json / javascript / python / text 等。
// 为简化只接受"json 系列"——本项目只产生 JSON 输出，其他语言标记视为异常。
func isFenceLangTag(s string) bool {
	switch strings.ToLower(s) {
	case "json":
		return true
	default:
		return false
	}
}