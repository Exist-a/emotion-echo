// Golden 测试：用 6 份真实 etc/*.yaml 验证 LoadBytes 能正确加载。
//
// 这些 yaml 全是大写风格（`Name: emotion-echo-chat-svc`、`SkyWalking.OAPAddr:` 等），
// 是 plan §四 4.1 R2 的证伪点：本测试通过 = A2 假设被实现覆盖，
// 不需要给 145 个 struct 字段加 yaml tag。
//
// 本测试只验证关键字段（Name/Port/Postgres.DSN/Nacos.Namespace 等）的"能加载且值正确"，
// 完整 struct 验证留到 PR-2~7 各服务切换时再做。
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findRepoRoot 从工作目录向上找 emotion-echo-shared 的父目录（仓库根）。
// 6 份 etc yaml 位于仓库根的 emotion-echo-<svc>/etc/ 目录。
//
// worktree 布局：worktree 本身就是仓库根的兄弟目录（路径 `D:\源码\emotion-echo-shared-refactor`），
// 所以 worktree 内 `../../<repo>/emotion-echo-<svc>/etc/` 不对；
// 正确做法：从 worktree 内部找 `emotion-echo-*/etc/*.yaml`。
//
// 测试在 worktree 内运行时，路径为 `emotion-echo-<svc>/etc/<svc>-api.yaml`。
func findRepoRoot(t *testing.T) string {
	t.Helper()
	// worktree 当前目录即仓库根（git worktree 的特点是 checkout 整份工作树）
	wd, err := os.Getwd()
	require.NoError(t, err)
	// sanity check: 应该有 go.work 或顶层 README.md
	if _, err := os.Stat(filepath.Join(wd, "emotion-echo-shared", "go.mod")); err == nil {
		return wd
	}
	// 兼容从子目录跑测试的情况
	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "emotion-echo-shared", "go.mod")); err == nil {
			return dir
		}
	}
	t.Fatal("findRepoRoot: 找不到 emotion-echo-shared 模块")
	return ""
}

// MinimalConfig 是所有 6 份 yaml 共有的最小字段集（plan §八 A2 关键验证）。
type MinimalConfig struct {
	Name string
	Host string
	Port int

	Postgres struct {
		DSN          string
		MaxOpenConns int
		MaxIdleConns int
	}
	Nacos struct {
		Addr      string
		Namespace string
		GroupName string
	}
}

func TestGolden_ChatSvc(t *testing.T) {
	root := findRepoRoot(t)
	var c MinimalConfig
	path := filepath.Join(root, "emotion-echo-chat-svc", "etc", "chat-api.yaml")
	err := Load(path, &c, func() {})
	require.NoError(t, err, "chat-api.yaml 必须能加载")

	assert.Equal(t, "emotion-echo-chat-svc", c.Name)
	assert.Equal(t, "0.0.0.0", c.Host)
	assert.Equal(t, 8890, c.Port)
	assert.Contains(t, c.Postgres.DSN, "emotion-echo-postgres", "Postgres.DSN 应含容器名")
	assert.Contains(t, c.Postgres.DSN, "search_path=emotion_echo_chat")
	assert.Equal(t, 10, c.Postgres.MaxOpenConns)
	assert.Equal(t, 5, c.Postgres.MaxIdleConns)
	assert.Equal(t, "emotion-echo-nacos:8848", c.Nacos.Addr)
	assert.Equal(t, "emotion-echo-dev", c.Nacos.Namespace)
	assert.Equal(t, "DEFAULT_GROUP", c.Nacos.GroupName)
}

func TestGolden_UserSvc(t *testing.T) {
	root := findRepoRoot(t)
	var c MinimalConfig
	path := filepath.Join(root, "emotion-echo-user-svc", "etc", "user-api.yaml")
	err := Load(path, &c, func() {})
	require.NoError(t, err, "user-api.yaml 必须能加载")

	assert.Equal(t, "emotion-echo-user-svc", c.Name)
	assert.Equal(t, 8888, c.Port)
	assert.Contains(t, c.Postgres.DSN, "search_path=emotion_echo_user")
	assert.Equal(t, "emotion-echo-dev", c.Nacos.Namespace)
}

func TestGolden_AssessmentSvc(t *testing.T) {
	root := findRepoRoot(t)
	var c MinimalConfig
	path := filepath.Join(root, "emotion-echo-assessment-svc", "etc", "assessment-api.yaml")
	err := Load(path, &c, func() {})
	require.NoError(t, err, "assessment-api.yaml 必须能加载")

	assert.Equal(t, "emotion-echo-assessment-svc", c.Name)
	assert.Equal(t, 8889, c.Port)
	assert.Contains(t, c.Postgres.DSN, "search_path=emotion_echo_assessment")
}

func TestGolden_AnalyticsSvc(t *testing.T) {
	root := findRepoRoot(t)
	var c MinimalConfig
	path := filepath.Join(root, "emotion-echo-analytics-svc", "etc", "analytics-api.yaml")
	err := Load(path, &c, func() {})
	require.NoError(t, err, "analytics-api.yaml 必须能加载")

	assert.Equal(t, "emotion-echo-analytics-svc", c.Name)
	assert.Equal(t, 8893, c.Port, "Stage 26-P:避开 ai-svc 8892")
	assert.Contains(t, c.Postgres.DSN, "search_path=emotion_echo_analytics")
}

// TestGolden_AISvc 验证 ai-svc 的复杂配置（LLM/GRPC/FER/SenseVoice/XTTS + ${...} 占位符字面值）。
// 这是 plan §六 R1/R2 双重风险叠加测试：
//   - R1 占位符字面值不应破坏加载（applyEnvOverrides 由 main.go 接管）
//   - R2 大写嵌套 key 必须能映射（A2 假设修正后已验证）
func TestGolden_AISvc(t *testing.T) {
	root := findRepoRoot(t)
	type AIConfig struct {
		Name string
		Port int
		LLM  struct {
			BaseURL string
			Model   string
			Enabled bool
		}
		GRPC struct {
			Enabled bool
			Port    int
		}
		FER struct {
			BaseURL string
		}
		Nacos struct {
			Addr      string
			Namespace string
		}
	}
	var c AIConfig
	path := filepath.Join(root, "emotion-echo-ai-svc", "etc", "ai-api.yaml")
	err := Load(path, &c, func() {})
	require.NoError(t, err, "ai-api.yaml 必须能加载（含占位符字面值）")

	assert.Equal(t, "emotion-echo-ai-svc", c.Name)
	assert.Equal(t, 8891, c.Port)
	assert.Contains(t, c.LLM.BaseURL, "${LLM_BASE_URL", "占位符字面值应原样保留，由 main.go applyEnvOverrides 接管")
	assert.Contains(t, c.LLM.BaseURL, "localhost:8000", "占位符里的默认值应保留在字面值里")
	assert.Equal(t, "${LLM_MODEL:-deepseek-chat}", c.LLM.Model, "ai-svc 占位符字面值原样保留")
	assert.True(t, c.LLM.Enabled, "LLM.Enabled: true 必须覆盖默认 false")
	assert.True(t, c.GRPC.Enabled)
	assert.Equal(t, 8892, c.GRPC.Port)
	assert.Equal(t, "", c.FER.BaseURL, "FER.BaseURL 显式空字符串应保留（dev 默认不调用）")
	assert.Equal(t, "${NACOS_ADDR:-emotion-echo-nacos:8848}", c.Nacos.Addr, "ai-svc 独有：占位符字面值原样保留，main.go applyEnvOverrides 接管")
	assert.Equal(t, "${NACOS_NAMESPACE:-emotion-echo-dev}", c.Nacos.Namespace, "同上")
}

// TestGolden_BFF 验证 BFF 配置（5 个下游 + Auth + TrustAPISIX + MinIO）。
func TestGolden_BFF(t *testing.T) {
	root := findRepoRoot(t)
	type BFFConfig struct {
		Name        string
		Port        int
		UserService struct {
			BaseURL string
		}
		ChatService struct {
			BaseURL string
		}
		AIService struct {
			HTTPAddr string
		}
		Auth struct {
			JWTSecret        string
			TokenTTLSeconds  int
		}
		TrustAPISIX bool
		MinIO       struct {
			Endpoint string
			Bucket   string
			UseSSL   bool
		}
	}
	var c BFFConfig
	path := filepath.Join(root, "emotion-echo-web-bff", "etc", "web-bff.yaml")
	err := Load(path, &c, func() {})
	require.NoError(t, err, "web-bff.yaml 必须能加载")

	assert.Equal(t, "emotion-echo-web-bff", c.Name)
	assert.Equal(t, 8894, c.Port)
	assert.Equal(t, "http://localhost:8888", c.UserService.BaseURL)
	assert.Equal(t, "http://localhost:8890", c.ChatService.BaseURL)
	assert.Equal(t, "http://localhost:8891", c.AIService.HTTPAddr)
	assert.Equal(t, "dev-bff-secret", c.Auth.JWTSecret)
	assert.Equal(t, 86400, c.Auth.TokenTTLSeconds)
	assert.True(t, c.TrustAPISIX)
	assert.Equal(t, "emotion-echo-minio:9000", c.MinIO.Endpoint)
	assert.Equal(t, "avatars", c.MinIO.Bucket)
	assert.False(t, c.MinIO.UseSSL)
}

// TestGolden_AllSvcsLoadable 是 catch-all 测试：6 份 yaml 全部能加载不报错。
// 任何 yaml 解析失败都会被这条测试捕获。
//
// 注意：BFF 没有 Postgres 字段（聚合层不直连 DB），所以只检查 Name/Port。
func TestGolden_AllSvcsLoadable(t *testing.T) {
	root := findRepoRoot(t)
	svcs := []struct {
		dir, file string
		hasDB     bool
	}{
		{"emotion-echo-user-svc", "user-api.yaml", true},
		{"emotion-echo-chat-svc", "chat-api.yaml", true},
		{"emotion-echo-assessment-svc", "assessment-api.yaml", true},
		{"emotion-echo-analytics-svc", "analytics-api.yaml", true},
		{"emotion-echo-ai-svc", "ai-api.yaml", true},
		{"emotion-echo-web-bff", "web-bff.yaml", false}, // BFF 聚合层，无 Postgres
	}
	for _, s := range svcs {
		t.Run(s.file, func(t *testing.T) {
			var c MinimalConfig
			path := filepath.Join(root, s.dir, "etc", s.file)
			err := Load(path, &c, func() {})
			require.NoError(t, err, "%s/%s 加载失败", s.dir, s.file)
			assert.NotEmpty(t, c.Name, "%s Name 应非空", s.file)
			assert.NotEmpty(t, c.Port, "%s Port 应非空", s.file)
			if s.hasDB {
				assert.NotEmpty(t, c.Postgres.DSN, "%s Postgres.DSN 应非空", s.file)
			}
		})
	}
}
