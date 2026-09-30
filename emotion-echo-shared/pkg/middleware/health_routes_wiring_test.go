package middleware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E-23 A 组 · 测试点 #8/#9 的配套守卫：readiness 端点必须真的在 6 个服务注册。
//
// 为什么不靠 handler 单测：handler 层证明"函数对"，
// 但证明不了"路由挂了"——把 main.go 里的 r.GET 行删掉，全部 handler
// 单测依然全绿。这类"接线缺失"只能从 main.go 源码层面钉。
//
// 读源码而非起真服务：六个服务的 main.go 需要真实 DB/Redis/Nacos 才能起，
// 单元测试层拿不到；而路由注册是纯文本可断言的事实。

// servicesWithHealthRoutes 列出应有 /health 与 /health/ready 的服务。
var servicesWithHealthRoutes = []string{
	"emotion-echo-user-svc",
	"emotion-echo-chat-svc",
	"emotion-echo-analytics-svc",
	"emotion-echo-assessment-svc",
	"emotion-echo-ai-svc",
	"emotion-echo-web-bff",
}

func TestHealthReadyRoute_RegisteredInAllServices(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("定位仓库根失败，跳过接线守卫：%v", err)
	}

	for _, svc := range servicesWithHealthRoutes {
		svc := svc
		t.Run(svc, func(t *testing.T) {
			t.Parallel()

			mainGo := filepath.Join(repoRoot, svc, "main.go")
			src, err := os.ReadFile(mainGo)
			if err != nil {
				t.Fatalf("读不到 %s：%v", mainGo, err)
			}
			content := string(src)

			assertContains := func(needle, why string) {
				t.Helper()
				if !strings.Contains(content, needle) {
					t.Errorf("%s 未在 main.go 注册 %q —— %s\n"+
						"（handler 单测仍会全绿，但端点不存在，compose 探针恒失败）",
						svc, needle, why)
				}
			}

			assertContains(`r.GET("/health"`, "liveness 端点")
			assertContains(`r.GET("/health/ready"`, "D-29 readiness 端点，compose healthcheck 依赖它")
		})
	}
}

// findRepoRoot 从本包位置向上找含 go.work 或多个服务目录的目录。
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "emotion-echo-user-svc", "main.go")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}
