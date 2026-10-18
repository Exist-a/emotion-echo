// Package logic — agentlogic_concurrency_test.go
//
// CL-06 · 第二方独立核对（2026-10-17）抓出的 **TOCTOU 竞态**回归钉。
//
// 竞态（核对结论，读码可判）：服务层原实现是 `CountActive()` → 判上限 → `Create()`，
// **三次独立往返、无事务、无行锁**，DDL 也无 CHECK/UNIQUE ⇒ 两个并发新建可双双读到
// `active=4` 再各插一条 ⇒ 第 6 个角色照样进库。默认角色物化同理（并发首次读 ⇒ 两条「小E」）。
//
// 修法：改走仓储的**原子**方法（`CreateWithActiveLimit` / `CreateIfNoHistory`），
// 其实现把"复核 + 插入"放进同一事务并先取**每用户咨询锁**。
//
// 本文件有**两层**钉子，强度不同，别混读：
//
//	① `TestAgentLimit_UsesAtomicRepoMethod`（**确定性**，真正的那道门）：断言服务层确实走
//	   仓储的**原子**方法。行为级并发测试在 InMemory 替身上**不能**证明竞态已修
//	   （窗口极小、时序相关 —— 本文件第一版就是"把实现改回旧写法后测试仍绿"，那等于没钉），
//	   实测教训：突变后不红 ⇒ 只能当 smoke，不能当证。
//	② 真并发（goroutine）两条：作为 smoke 与"上限内都成功"的合理性检查保留。
package logic

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 并发新建：无论起多少并发，活跃角色数**不得超过上限**
func TestUpsertAgentProfile_ConcurrentCreates_NeverExceedLimit(t *testing.T) {
	t.Parallel()
	svcCtx, repo := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	const workers = 12
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			req := validUpsert(fmt.Sprintf("并发%d", i))
			_, _ = NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(req)
		}(i)
	}
	wg.Wait()

	active, err := repo.CountActive(context.Background(), 1)
	require.NoError(t, err)
	assert.LessOrEqual(t, active, MaxAgentProfilesPerUser,
		"%d 个并发新建后活跃角色数必须 ≤ %d（否则是 Count→Create 的 TOCTOU 回来了）", workers, MaxAgentProfilesPerUser)
	assert.Equal(t, MaxAgentProfilesPerUser, active, "上限内应当都插入成功（不是靠大量失败换来的）")
}

// 并发首次读：默认角色**只能物化一条**
func TestListAgentProfiles_ConcurrentFirstReads_SeedExactlyOneDefault(t *testing.T) {
	t.Parallel()
	svcCtx, repo := newAgentLogicEnv(t)
	ctx := agentCtx(2)

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, _ = NewListAgentProfilesLogic(ctx, svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
		}()
	}
	wg.Wait()

	items, err := repo.ListByUserID(context.Background(), 2)
	require.NoError(t, err)

	defaults := 0
	for _, it := range items {
		if it.IsDefault {
			defaults++
		}
	}
	assert.Equal(t, 1, defaults,
		"并发首次读只能物化 1 条默认角色（原实现并发下会物化多条 —— 见本文件头注释）")
	assert.Len(t, items, 1, "只应有那一条默认角色")
}

// recordingRepo 记录服务层实际调用了哪个仓储方法（确定性接线的抓手）。
type recordingRepo struct {
	*repository.InMemoryUserAgentProfileRepo
	mu              sync.Mutex
	usedAtomicLimit bool
	usedAtomicSeed  bool
	usedLegacyCount bool
}

func (r *recordingRepo) CreateWithActiveLimit(ctx context.Context, p *model.UserAgentProfile, maxActive int) error {
	r.mu.Lock()
	r.usedAtomicLimit = true
	r.mu.Unlock()
	return r.InMemoryUserAgentProfileRepo.CreateWithActiveLimit(ctx, p, maxActive)
}

func (r *recordingRepo) CreateIfNoHistory(ctx context.Context, p *model.UserAgentProfile) (bool, error) {
	r.mu.Lock()
	r.usedAtomicSeed = true
	r.mu.Unlock()
	return r.InMemoryUserAgentProfileRepo.CreateIfNoHistory(ctx, p)
}

func (r *recordingRepo) CountActive(ctx context.Context, userID int64) (int, error) {
	r.mu.Lock()
	r.usedLegacyCount = true
	r.mu.Unlock()
	return r.InMemoryUserAgentProfileRepo.CountActive(ctx, userID)
}

func (r *recordingRepo) Create(ctx context.Context, p *model.UserAgentProfile) (*model.UserAgentProfile, error) {
	// 服务层不应再直接用裸 Create 来建角色（那会绕开上限复核）
	r.mu.Lock()
	r.usedLegacyCount = true
	r.mu.Unlock()
	return r.InMemoryUserAgentProfileRepo.Create(ctx, p)
}

// 确定性钉子：上限与物化都**必须**走原子方法。
// （为什么这条才是真钉：并发行为测试在 InMemory 上可能"因为没撞上时序"而假绿 ——
//
//	第一版就是这样：把实现改回 Count+Create 后它仍然 PASS。突变实验是判据。）
func TestAgentLimit_UsesAtomicRepoMethod(t *testing.T) {
	t.Parallel()
	repo := &recordingRepo{InMemoryUserAgentProfileRepo: repository.NewInMemoryUserAgentProfileRepo()}
	svcCtx := svc.NewServiceContext(config.Config{}, nil, nil, repo)
	ctx := agentCtx(3)

	// 新建一个角色 ⇒ 必须走 CreateWithActiveLimit
	_, err := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(validUpsert("阿哲"))
	require.NoError(t, err)
	assert.True(t, repo.usedAtomicLimit,
		"新建角色必须走 CreateWithActiveLimit（事务+咨询锁内复核）；走 CountActive+Create 会留下 TOCTOU")

	// 首次读列表 ⇒ 必须走 CreateIfNoHistory
	repo2 := &recordingRepo{InMemoryUserAgentProfileRepo: repository.NewInMemoryUserAgentProfileRepo()}
	svcCtx2 := svc.NewServiceContext(config.Config{}, nil, nil, repo2)
	_, err = NewListAgentProfilesLogic(agentCtx(4), svcCtx2).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	assert.True(t, repo2.usedAtomicSeed,
		"默认角色物化必须走 CreateIfNoHistory（否则并发首次读会物化出多条默认角色）")
}
