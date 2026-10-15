// CL-05 Phase 5（2026-10-16）：会话列表游标的**编码 / 解码**。
//
// 为什么游标是**复合键**而不是单个 `updated_at`：
//
//	会话列表的稳定序是 `updated_at DESC, id DESC`；而**同一时刻**更新的多个会话
//	（同一事务的 NOW() 取事务开始时间 ⇒ 同值；种子数据也常同值）`updated_at` **完全相同**。
//	若游标只带 updated_at，下一页条件就是 `updated_at < cursor` —— 对这批同刻行**恒假**
//	⇒ 它们会被**整批跳过**（漏行）。带上 id 做 tie-break
//	（`updated_at = cursor AND id < cursor_id`）才不重不漏。
//
// 编码格式 `<unixMicros>:<id>`。**为什么是微秒**：PG timestamptz 的精度是微秒；
// 用毫秒会**截断** —— 于是"同刻行"的数量反而变多，等于把上面这个 bug 换了个形状
//	（截断后的 cursor 既不 `<` 也不 `=` 真实值 ⇒ 连 tie-break 都失效）。
//
// 该编码**由 chat-svc 独占**：BFF 与前端只做**原样回传**（不透明游标），
// 因此格式变更不会外溢到其他层。
package logic

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const conversationCursorSep = ":"

// encodeConversationCursor 把 (updated_at, id) 编成不透明游标；零值 ⇒ 空串（无下一页）。
func encodeConversationCursor(updatedAt time.Time, id int64) string {
	if updatedAt.IsZero() && id == 0 {
		return ""
	}
	return fmt.Sprintf("%d%s%d", updatedAt.UnixMicro(), conversationCursorSep, id)
}

// decodeConversationCursor 解码游标；空串 ⇒ 零值 + id 0（= 首页），**不是**错误。
// 非法格式返回错误（由 handler 映射成 400），避免静默退化成"首页"而让翻页悄悄重来。
func decodeConversationCursor(s string) (time.Time, int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, 0, nil
	}
	parts := strings.Split(s, conversationCursorSep)
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid conversation cursor %q: want <unixMicros>:<id>", s)
	}
	us, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid conversation cursor %q: bad timestamp part", s)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid conversation cursor %q: bad id part", s)
	}
	return time.UnixMicro(us).UTC(), id, nil
}
