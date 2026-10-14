// Package handler — personality_contract_test.go
//
// CL-04 · Phase 4（T-9）：人格展示契约的**三方一致性守卫**。
//
// 为什么需要：同一份契约由三处独立实现 —— 前端展示（`personality.ts`）/ BFF 提示词
// （`personality_directive.go` 的适配表与阈值）/ assessment 计分（`scorer.go` 的 dimensionMap）。
// 前端那份的**文件头自己就写着**"与后端两处实现保持一致，改任一处必须同步改另两处"
// —— 纯靠人记；本项目历史上正是这类"人工同步"漂移过（同一实体三处映射漂移）。
//
// 为什么用"一致性守卫"而不是"单一来源派生"（plan §3.4 的二选一）：
// 前端要在**运行时**用 label 渲染雷达图，派生要么加接口要么上 codegen；
// 先上机器守卫把漂移变成红灯，成本最低、收益直接。
//
// 为什么放在 Go 测试而不是 `scripts/` 守卫：Go 测试随 `go-test.yml` 自动跑，
// **不用改 `.github/workflows/**`**（那是 D 轨文件面，且新 job 要动总闸 needs）。
// CI 是整仓 checkout（`defaults.run.working-directory` 只切目录）⇒ 这里的相对路径在 CI 同样成立。
//
// 反"静默跳过"纪律：解析不到就**判红**，绝不"未解析到 ⇒ 跳过"（E2E-F-180 教训）。
package handler

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	frontendPersonalityPath = "../../../emotion-echo-web/app/configs/personality.ts"
	assessmentScorerPath    = "../../../emotion-echo-assessment-svc/internal/scoring/scorer.go"

	expectedPersonalityDims     = 5
	expectedQuestionsPerDim     = 6
	expectedPersonalityMaxScore = 30
)

// personalityContract 三方对同一份展示契约的投影（缺项用零值，diff 时会被发现）。
type personalityContract struct {
	order           []string          // 维度顺序（顺序即展示/输出顺序，确定性）
	labels          map[string]string // 维度键 → 中文标签
	minScore        int               // 单维最低分（文案声明）
	maxScore        int               // 单维满分（文案声明 / 前端常量）
	highThreshold   int               // 高档阈值（≥ 为高）
	lowThreshold    int               // 低档阈值（≤ 为低）
	questionsPerDim map[string]int    // 每维题数（用于核对 maxScore = 题数 × 5）
}

func readFileOrFail(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err, "守卫依赖该文件存在（路径变动须同步改守卫）：%s", path)
	return string(b)
}

// bffPersonalityContract 读 BFF 自己的人格契约：适配表（同包常量）+ **文案里的分数区间**
// （`每维 6-30` 是给模型看的声明，与前端满分 30 必须一致 —— 这正是"文案数字 vs 代码契约"易漂移处）。
func bffPersonalityContract(t *testing.T) personalityContract {
	t.Helper()
	c := personalityContract{
		labels:          map[string]string{},
		questionsPerDim: map[string]int{},
		highThreshold:   personalityHighThreshold,
		lowThreshold:    personalityLowThreshold,
	}
	for _, g := range personalityStyleGuides {
		c.order = append(c.order, g.Key)
		c.labels[g.Key] = g.Label
	}

	src := readFileOrFail(t, personalityTemplatesFile)
	m := reBFFScoreRange.FindStringSubmatch(src)
	require.Len(t, m, 3, "未解析到 BFF 文案里的「每维 N-M」区间声明（守卫须同步改，不得静默跳过）")
	c.minScore = mustAtoi(t, m[1])
	c.maxScore = mustAtoi(t, m[2])
	return c
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	require.NoError(t, err, "整数解析失败: %q", s)
	return n
}

var (
	reFrontendDim     = regexp.MustCompile(`\{\s*key:\s*'([a-z_]+)',\s*label:\s*'([^']+)'\s*\}`)
	reFrontendMax     = regexp.MustCompile(`PERSONALITY_DIMENSION_MAX\s*=\s*(\d+)`)
	reFrontendHigh    = regexp.MustCompile(`score\s*>=\s*(\d+)`)
	reFrontendLow     = regexp.MustCompile(`score\s*<=\s*(\d+)`)
	reAssessmentDim   = regexp.MustCompile(`"([a-z_]+)":\s*\{([^}]*)\}`)
	reQuotedQ         = regexp.MustCompile(`"q\d+"`)
	reBFFScoreRange   = regexp.MustCompile(`每维\s*(\d+)-(\d+)`)
)

func parseFrontendPersonalityContract(t *testing.T, src string) personalityContract {
	t.Helper()
	c := personalityContract{labels: map[string]string{}, questionsPerDim: map[string]int{}}

	for _, m := range reFrontendDim.FindAllStringSubmatch(src, -1) {
		c.order = append(c.order, m[1])
		c.labels[m[1]] = m[2]
	}
	require.Len(t, c.order, expectedPersonalityDims,
		"未解析到 %d 个维度（前端契约结构变了？守卫须同步改，不得静默跳过）", expectedPersonalityDims)

	c.maxScore = mustInt(t, reFrontendMax, src, "PERSONALITY_DIMENSION_MAX")
	c.highThreshold = mustInt(t, reFrontendHigh, src, "score >= N")
	c.lowThreshold = mustInt(t, reFrontendLow, src, "score <= N")
	return c
}

func parseAssessmentPersonalityContract(t *testing.T, src string) personalityContract {
	t.Helper()
	c := personalityContract{labels: map[string]string{}, questionsPerDim: map[string]int{}}

	for _, m := range reAssessmentDim.FindAllStringSubmatch(src, -1) {
		c.order = append(c.order, m[1])
		c.questionsPerDim[m[1]] = len(reQuotedQ.FindAllString(m[2], -1))
	}
	require.Len(t, c.order, expectedPersonalityDims,
		"未解析到 %d 个维度（assessment dimensionMap 结构变了？守卫须同步改）", expectedPersonalityDims)
	return c
}

func mustInt(t *testing.T, re *regexp.Regexp, src, what string) int {
	t.Helper()
	m := re.FindStringSubmatch(src)
	require.Len(t, m, 2, "未解析到 %s（前端契约结构变了？守卫须同步改）", what)
	n, err := strconv.Atoi(m[1])
	require.NoError(t, err, "%s 解析失败", what)
	return n
}

// diffPersonalityContracts 返回两方投影的差异描述（空 = 一致）。
func diffPersonalityContracts(nameA string, a personalityContract, nameB string, b personalityContract) []string {
	var out []string
	if got, want := strings.Join(b.order, ","), strings.Join(a.order, ","); got != want {
		out = append(out, nameA+" 维度序列="+want+" ≠ "+nameB+" 维度序列="+got)
	}
	for _, k := range a.order {
		if av, bv := a.labels[k], b.labels[k]; bv != "" && av != bv {
			out = append(out, nameA+" 标签["+k+"]="+av+" ≠ "+nameB+" 标签["+k+"]="+bv)
		}
		if av, bv := a.questionsPerDim[k], b.questionsPerDim[k]; av != 0 && bv != 0 && av != bv {
			out = append(out, nameA+" 题数["+k+"]="+strconv.Itoa(av)+" ≠ "+nameB+" 题数["+k+"]="+strconv.Itoa(bv))
		}
		if av, bv := a.maxScore, b.maxScore; av != 0 && bv != 0 && av != bv {
			out = append(out, nameA+" 满分="+strconv.Itoa(av)+" ≠ "+nameB+" 满分="+strconv.Itoa(bv))
		}
		if av, bv := a.highThreshold, b.highThreshold; av != 0 && bv != 0 && av != bv {
			out = append(out, nameA+" 高档阈值="+strconv.Itoa(av)+" ≠ "+nameB+" 高档阈值="+strconv.Itoa(bv))
		}
		if av, bv := a.lowThreshold, b.lowThreshold; av != 0 && bv != 0 && av != bv {
			out = append(out, nameA+" 低档阈值="+strconv.Itoa(av)+" ≠ "+nameB+" 低档阈值="+strconv.Itoa(bv))
		}
	}
	return out
}

// T-9 主体：三方一致
func TestPersonalityContract_ThreeWayConsistent(t *testing.T) {
	bff := bffPersonalityContract(t)
	fe := parseFrontendPersonalityContract(t, readFileOrFail(t, frontendPersonalityPath))
	as := parseAssessmentPersonalityContract(t, readFileOrFail(t, assessmentScorerPath))

	// 前置健全性：BFF 侧自身必须先自洽（否则守卫在比"错的基准"）
	require.Len(t, bff.order, expectedPersonalityDims, "BFF 适配表维度数异常")

	// 解析器自检（防空跑）：用已知样本确认正则真的能抓
	require.Equal(t, 23, mustInt(t, reFrontendHigh, "  if (score >= 23) return '高'", "self-check"),
		"解析器自检失败：正则抓不到分档阈值")
	require.Equal(t, 6, mustAtoi(t, reBFFScoreRange.FindStringSubmatch("每维 6-30，18 为中性")[1]), "解析器自检失败")

	assert.Empty(t, diffPersonalityContracts("BFF", bff, "前端", fe),
		"前端 personality.ts 与 BFF 的人格契约漂移（前端文件头要求人工同步，本守卫把它变成红灯）")
	assert.Empty(t, diffPersonalityContracts("BFF", bff, "assessment", as),
		"assessment scorer 的维度映射与 BFF 漂移")

	// 满分派生核对：assessment 每维题数 × 5 必须等于前端声明的满分
	for k, n := range as.questionsPerDim {
		assert.Equal(t, expectedQuestionsPerDim, n, "维度 %s 题数异常", k)
		assert.Equal(t, expectedPersonalityMaxScore, n*5,
			"满分 %d 必须等于 %s 的题数 %d × 5", expectedPersonalityMaxScore, k, n)
	}
	assert.Equal(t, expectedPersonalityMaxScore, fe.maxScore, "前端声明的满分异常")
}

// T-9 负向对照：守卫必须真的能抓到漂移（否则就是"永远绿的摆设"）
func TestPersonalityContract_DriftIsDetected(t *testing.T) {
	fe := parseFrontendPersonalityContract(t, readFileOrFail(t, frontendPersonalityPath))
	bff := bffPersonalityContract(t)

	mutated := fe
	mutated.labels = map[string]string{}
	for k, v := range fe.labels {
		mutated.labels[k] = v
	}
	mutated.labels[bff.order[0]] = "改了标签"
	assert.NotEmpty(t, diffPersonalityContracts("BFF", bff, "前端", mutated), "标签漂移必须被抓到")

	mutated2 := fe
	mutated2.highThreshold = fe.highThreshold + 1
	assert.NotEmpty(t, diffPersonalityContracts("BFF", bff, "前端", mutated2), "阈值漂移必须被抓到")

	mutated3 := fe
	mutated3.order = append([]string{fe.order[1]}, append(fe.order[:1], fe.order[2:]...)...)
	assert.NotEmpty(t, diffPersonalityContracts("BFF", bff, "前端", mutated3), "维度顺序漂移必须被抓到")
}
