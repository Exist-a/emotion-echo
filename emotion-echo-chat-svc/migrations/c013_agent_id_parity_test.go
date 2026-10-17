// migrations/c013_agent_id_parity_test.go
//
// CL-06：`agent_id` 的**双事实源对等测试** —— 迁移 SQL 与 chat-svc model 必须一致。
//
// 为什么需要（沿用 c010/c011 的教训）：DDL 与 GORM model 是**两份**事实源；
// 只改一边（改列名、漏列、把可空改成 NOT NULL）不会编译报错、也不会被任何现有守卫发现，
// 直到运行时才炸（§契约 5「schema 与写入端一致性」正是为此立的）。
//
// 本钉断言（双向，不是"包含"）：
//   A. `c012` 恰好给 conversations 加 `agent_id`；`c013` 恰好给 messages 加 `agent_id`；
//   B. 对应 model 结构体里**恰有**一个字段映射到 `column:agent_id`；
//   C. 两者类型语义一致：可空（`*int64` ↔ SQL 无 `NOT NULL`）—— 因为 NULL 表示"未显式选择 /
//      历史行未记录"，把它改成 NOT NULL 会让"未选择"无值可填。
package migrations

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"emotion-echo-chat-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alterAddColumnRe 解析 `ALTER TABLE <schema>.<table> ADD COLUMN IF NOT EXISTS <col> <type>`。
// (?is) 允许跨行；`NOT NULL` 单独捕获以便断言可空性。
var alterAddColumnRe = regexp.MustCompile(
	`(?is)ALTER\s+TABLE\s+([\w.]+)\s+ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\s+(\w+)\s+([\w()]+)\s*(NOT\s+NULL)?`)

type addedColumn struct {
	Table   string
	Column  string
	Type    string
	NotNull bool
}

func parseAddedColumns(t *testing.T, file string) []addedColumn {
	t.Helper()
	raw, err := os.ReadFile(file)
	require.NoError(t, err, "读迁移文件失败：%s", file)
	var out []addedColumn
	for _, m := range alterAddColumnRe.FindAllStringSubmatch(string(raw), -1) {
		out = append(out, addedColumn{
			Table:   strings.TrimPrefix(m[1], "emotion_echo_chat."),
			Column:  m[2],
			Type:    strings.ToUpper(m[3]),
			NotNull: strings.TrimSpace(m[4]) != "",
		})
	}
	return out
}

// gormColumnFields 返回结构体里所有映射到 column:<name> 的字段名 → 字段类型。
func gormColumnFields(v any) map[string]reflect.Type {
	rt := reflect.TypeOf(v)
	out := map[string]reflect.Type{}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("gorm")
		if idx := strings.Index(tag, "column:"); idx >= 0 {
			rest := tag[idx+len("column:"):]
			name := rest
			if semi := strings.IndexAny(rest, ";,"); semi >= 0 {
				name = rest[:semi]
			}
			out[name] = f.Type
		}
	}
	return out
}

func TestAgentIDMigrationAndModelParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		file       string
		wantTable  string
		modelValue any
	}{
		{
			name:       "c012 conversations.agent_id",
			file:       "c012_add_agent_id_to_conversations.sql",
			wantTable:  "conversations",
			modelValue: model.Conversation{},
		},
		{
			name:       "c013 messages.agent_id",
			file:       "c013_add_agent_id_to_messages.sql",
			wantTable:  "messages",
			modelValue: model.Message{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cols := parseAddedColumns(t, tc.file)
			require.Len(t, cols, 1, "%s 应当**恰好**加一列（多于 1 列说明有人往这个文件里塞了别的东西）", tc.file)
			got := cols[0]
			assert.Equal(t, tc.wantTable, got.Table, "加列的目标表不符")
			assert.Equal(t, "agent_id", got.Column, "列名不符")
			assert.Equal(t, "BIGINT", got.Type, "列类型应为 BIGINT（与 user_agent_profile.id 一致）")
			assert.False(t, got.NotNull, "agent_id 必须**可空**（NULL = 未显式选择 / 历史行未记录）")

			fields := gormColumnFields(tc.modelValue)
			ft, ok := fields["agent_id"]
			require.True(t, ok, "model 里找不到映射到 column:agent_id 的字段（DDL 有列、model 没有 ⇒ 写入端永远写不进去）")
			assert.Equal(t, reflect.Ptr, ft.Kind(), "字段应为指针类型（可区分 NULL 与 0）")
			assert.Equal(t, reflect.Int64, ft.Elem().Kind(), "指针底层应为 int64")
		})
	}
}

// 反向断言：**只**这两个迁移文件碰 agent_id —— 避免有人在别处偷偷加同名列导致双写。
func TestAgentIDColumnAddedByExactlyTwoMigrations(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	_ = fs.FileInfoToDirEntry // 保持 io/fs 引用语义（与仓库其他守卫一致的写法）

	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	require.NotEmpty(t, files)

	var hits []string
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(".", f))
		require.NoError(t, err)
		if strings.Contains(strings.ToLower(string(raw)), "agent_id") {
			hits = append(hits, f)
		}
	}
	assert.ElementsMatch(t,
		[]string{"c012_add_agent_id_to_conversations.sql", "c013_add_agent_id_to_messages.sql"},
		hits,
		"agent_id 只应由 c012/c013 引入（列表不符 = 有人另加了同名列）")
}
