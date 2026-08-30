// Package repository 定义 analytics-svc 的数据访问层
package repository

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"emotion-echo-analytics-svc/internal/model"

	"gorm.io/gorm"
)

// EventRepo 行为事件仓储接口
//
// Stage 30-A Round 2 扩展：增加 user-behavior 查询方法（day-night 桶、
// interaction depth、frequency trend）。这些是 read-only 聚合，仍由
// analytics_reader role 走 search_path。
type EventRepo interface {
	GetByID(ctx context.Context, id int64) (*model.UserBehaviorEvent, error)
	Create(ctx context.Context, e *model.UserBehaviorEvent) error

	// GetDayNightPattern 按 24 小时桶聚合指定用户在 [start, end] 内
	// 的事件。hour=0..23 → count。返回桶必须 len=24（缺失的 hour 计为 0）。
	GetDayNightPattern(ctx context.Context, userID int64, start, end time.Time) (map[int]int64, error)

	// GetInteractionDepth 返回 [start, end] 窗口内用户活跃度指标：
	//   totalMessages  消息总数
	//   totalConversations  会话总数
	//   avgMessagesPerConv  平均会话消息数数（保留 2 位小数）
	//   longestConversationMs  最长单次会话跨度（毫秒）
	GetInteractionDepth(ctx context.Context, userID int64, start, end time.Time) (*InteractionDepth, error)

	// GetFrequencyTrend 返回 [start, end] 内按天聚合的事件总数
	// （daily granularity，窗口最大 90 天）。
	GetFrequencyTrend(ctx context.Context, userID int64, start, end time.Time) ([]DailyCount, error)

	Ping(ctx context.Context) error
}

// InteractionDepth 用户交互深度指标
type InteractionDepth struct {
	TotalMessages           int64   `json:"totalMessages"`
	TotalConversations      int64   `json:"totalConversations"`
	AvgMessagesPerConv      float64 `json:"avgMessagesPerConv"`
	LongestConversationMs  int64   `json:"longestConversationMs"`
}

// DailyCount 单日事件计数
type DailyCount struct {
	Date  string `json:"date"`  // YYYY-MM-DD
	Count int64  `json:"count"`
}

var ErrNotFound = errors.New("analytics: event not found")

type InMemoryEventRepo struct {
	mu     sync.RWMutex
	data   map[int64]*model.UserBehaviorEvent
	nextID int64
}

func NewInMemoryEventRepo() *InMemoryEventRepo {
	return &InMemoryEventRepo{data: make(map[int64]*model.UserBehaviorEvent), nextID: 1}
}

func (r *InMemoryEventRepo) GetByID(ctx context.Context, id int64) (*model.UserBehaviorEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.data[id]; ok {
		return e, nil
	}
	return nil, nil
}

func (r *InMemoryEventRepo) Create(ctx context.Context, e *model.UserBehaviorEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == 0 {
		e.ID = r.nextID
		r.nextID++
	}
	r.data[e.ID] = e
	return nil
}

// GetDayNightPattern 实现：按 EventType='message' 计算 hour 桶。
//
// 内存实现保留完整 Round 2 行为（无未来时区歧义），逻辑层只
// 负责补 0 bucket。
func (r *InMemoryEventRepo) GetDayNightPattern(_ context.Context, userID int64, start, end time.Time) (map[int]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[int]int64{}
	for _, e := range r.data {
		if e.UserID != userID {
			continue
		}
		if e.OccurredAt.Before(start) || e.OccurredAt.After(end.Add(24*time.Hour-time.Nanosecond)) {
			continue
		}
		out[e.OccurredAt.Hour()]++
	}
	return out, nil
}

// GetInteractionDepth 实现：消息数 / 会话数 / 最长会话 ms。
//
// 内存实现按 SessionID 分组；最简近似（不做窗口合并）。
func (r *InMemoryEventRepo) GetInteractionDepth(_ context.Context, userID int64, start, end time.Time) (*InteractionDepth, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var (
		total      int64
		convCount  = map[string]struct{}{}
		convRanges = map[string][2]time.Time{}
	)
	for _, e := range r.data {
		if e.UserID != userID {
			continue
		}
		if e.OccurredAt.Before(start) || e.OccurredAt.After(end.Add(24*time.Hour-time.Nanosecond)) {
			continue
		}
		total++
		if e.SessionID != "" {
			convCount[e.SessionID] = struct{}{}
			rng, ok := convRanges[e.SessionID]
			if !ok {
				rng = [2]time.Time{e.OccurredAt, e.OccurredAt}
			} else {
				if e.OccurredAt.Before(rng[0]) {
					rng[0] = e.OccurredAt
				}
				if e.OccurredAt.After(rng[1]) {
					rng[1] = e.OccurredAt
				}
			}
			convRanges[e.SessionID] = rng
		}
	}
	convs := int64(len(convCount))
	var avg float64
	if convs > 0 {
		avg = float64(total) / float64(convs)
	}
	var longestMs int64
	for _, rng := range convRanges {
		ms := rng[1].Sub(rng[0]).Milliseconds()
		if ms > longestMs {
			longestMs = ms
		}
	}
	return &InteractionDepth{
		TotalMessages:          total,
		TotalConversations:     convs,
		AvgMessagesPerConv:     avg,
		LongestConversationMs: longestMs,
	}, nil
}

// GetFrequencyTrend 实现：按 YYYY-MM-DD 桶聚合事件数。
func (r *InMemoryEventRepo) GetFrequencyTrend(_ context.Context, userID int64, start, end time.Time) ([]DailyCount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	buckets := map[string]int64{}
	for _, e := range r.data {
		if e.UserID != userID {
			continue
		}
		if e.OccurredAt.Before(start) || e.OccurredAt.After(end.Add(24*time.Hour-time.Nanosecond)) {
			continue
		}
		buckets[e.OccurredAt.Format("2006-01-02")]++
	}
	out := make([]DailyCount, 0, len(buckets))
	for d, c := range buckets {
		out = append(out, DailyCount{Date: d, Count: c})
	}
	return out, nil
}

func (r *InMemoryEventRepo) Ping(ctx context.Context) error { return nil }

type PostgresEventRepo struct{ db *gorm.DB }

func NewPostgresEventRepo(db *gorm.DB) *PostgresEventRepo {
	return &PostgresEventRepo{db: db}
}

func (r *PostgresEventRepo) GetByID(ctx context.Context, id int64) (*model.UserBehaviorEvent, error) {
	var e model.UserBehaviorEvent
	err := r.db.WithContext(ctx).First(&e, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (r *PostgresEventRepo) Create(ctx context.Context, e *model.UserBehaviorEvent) error {
	e.ID = 0
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *PostgresEventRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// GetDayNightPattern 按 24 小时桶聚合 [start, end]（含 end 当日）内的事件。
//
// 窗口语义与 InMemory 实现一致：[start::date, end::date + 1 day)。
// 日期边界以 YYYY-MM-DD 字符串传入 SQL（`$n::date`），避免 timestamptz→date
// 依赖数据库会话时区。返回稀疏 map（缺失的 hour 不出现），logic 层补满 24 桶。
// 时间桶按数据库会话时区取 EXTRACT(HOUR)（生产 UTC，测试容器默认 UTC）。
func (r *PostgresEventRepo) GetDayNightPattern(ctx context.Context, userID int64, start, end time.Time) (map[int]int64, error) {
	const q = `
SELECT EXTRACT(HOUR FROM occurred_at)::int AS hour, COUNT(*)::bigint AS cnt
FROM emotion_echo_analytics.user_behavior_events
WHERE user_id = $1
  AND occurred_at >= $2::date
  AND occurred_at < ($3::date + INTERVAL '1 day')
GROUP BY 1`

	var rows []struct {
		Hour int
		Cnt  int64
	}
	if err := r.db.WithContext(ctx).Raw(q, userID, start.Format("2006-01-02"), end.Format("2006-01-02")).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[int]int64, len(rows))
	for _, row := range rows {
		out[row.Hour] = row.Cnt
	}
	return out, nil
}

// GetInteractionDepth 计算 [start, end] 内用户活跃度指标。
//
// 语义（与 InMemory 近似一致）：
//   - totalMessages = 窗口内事件总数
//   - totalConversations = DISTINCT session_id 数
//   - avgMessagesPerConv = total / convs（0 会话时 0，保留 2 位小数）
//   - longestConversationMs = 按 session_id 分组的首末事件跨度的最大值
func (r *PostgresEventRepo) GetInteractionDepth(ctx context.Context, userID int64, start, end time.Time) (*InteractionDepth, error) {
	const q = `
WITH windowed AS (
    SELECT session_id, occurred_at
    FROM emotion_echo_analytics.user_behavior_events
    WHERE user_id = $1
      AND occurred_at >= $2::date
      AND occurred_at < ($3::date + INTERVAL '1 day')
),
session_agg AS (
    SELECT session_id,
           MIN(occurred_at) AS min_at,
           MAX(occurred_at) AS max_at
    FROM windowed
    GROUP BY session_id
)
SELECT
    (SELECT COUNT(*)::bigint FROM windowed)                AS total_messages,
    (SELECT COUNT(*)::bigint FROM session_agg)             AS total_conversations,
    COALESCE((SELECT MAX(EXTRACT(EPOCH FROM (max_at - min_at)) * 1000)
              FROM session_agg), 0)::bigint                AS longest_ms`

	var row struct {
		TotalMessages      int64
		TotalConversations int64
		LongestMs          int64
	}
	if err := r.db.WithContext(ctx).Raw(q, userID, start.Format("2006-01-02"), end.Format("2006-01-02")).Scan(&row).Error; err != nil {
		return nil, err
	}

	var avg float64
	if row.TotalConversations > 0 {
		avg = float64(row.TotalMessages) / float64(row.TotalConversations)
	}
	return &InteractionDepth{
		TotalMessages:          row.TotalMessages,
		TotalConversations:     row.TotalConversations,
		AvgMessagesPerConv:     math.Round(avg*100) / 100,
		LongestConversationMs:  row.LongestMs,
	}, nil
}

// GetFrequencyTrend 返回 [start, end] 内按天聚合的事件计数（daily 粒度，升序）。
func (r *PostgresEventRepo) GetFrequencyTrend(ctx context.Context, userID int64, start, end time.Time) ([]DailyCount, error) {
	const q = `
SELECT occurred_at::date AS day, COUNT(*)::bigint AS cnt
FROM emotion_echo_analytics.user_behavior_events
WHERE user_id = $1
  AND occurred_at >= $2::date
  AND occurred_at < ($3::date + INTERVAL '1 day')
GROUP BY 1
ORDER BY 1`

	var rows []struct {
		Day time.Time
		Cnt int64
	}
	if err := r.db.WithContext(ctx).Raw(q, userID, start.Format("2006-01-02"), end.Format("2006-01-02")).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DailyCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, DailyCount{Date: row.Day.Format("2006-01-02"), Count: row.Cnt})
	}
	return out, nil
}