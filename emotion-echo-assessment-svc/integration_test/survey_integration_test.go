//go:build integration
// +build integration

// Package integration_test 真实 Postgres + assessment-svc SurveyRepo CRUD + HealthLogic。
//
// 流程：testcontainers postgres + emotion_echo_assessment schema + surveys/survey_results 表
//        → 真实 PostgresSurveyRepo + scoring scorer
//        → 走 GetByCode / SaveResult / GetResult / ListResultsByUser 全路径
//
// 跑：  go test -tags integration -v -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"emotion-echo-assessment-svc/internal/model"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/scoring"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// pgContainerDesc 起 postgres + surveys/survey_results 表
func pgContainerDesc(t *testing.T, ctx context.Context) (*pgcontainer.PostgresContainer, *gorm.DB) {
	t.Helper()

	pgC, err := pgcontainer.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		pgcontainer.WithDatabase("emotion_echo_test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, runSQL(ctx, dsn, `CREATE SCHEMA IF NOT EXISTS emotion_echo_assessment`))
	// surveys 表（minimal）
	require.NoError(t, runSQL(ctx, dsn, `
CREATE TABLE IF NOT EXISTS emotion_echo_assessment.surveys (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(64) UNIQUE NOT NULL,
  title VARCHAR(255),
  description TEXT,
  category VARCHAR(32),
  questions JSONB DEFAULT '{}',
  scoring_rules JSONB DEFAULT '{}',
  version INT DEFAULT 1,
  status SMALLINT DEFAULT 1,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
)`))
	require.NoError(t, runSQL(ctx, dsn, `
CREATE TABLE IF NOT EXISTS emotion_echo_assessment.survey_results (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  survey_id BIGINT NOT NULL,
  answers JSONB DEFAULT '{}',
  factor_scores JSONB DEFAULT '{}',
  total_score REAL DEFAULT 0,
  risk_level VARCHAR(32),
  duration_seconds INT DEFAULT 0,
  submitted_at TIMESTAMPTZ DEFAULT NOW()
)`))

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	return pgC, db
}

func runSQL(ctx context.Context, dsn, sql string) error {
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Exec(sql).Error
}

// TestAssessment_Integration_PostgresSurveyRepo 真实 Postgres 写入 + 读出 + 列
func TestAssessment_Integration_PostgresSurveyRepo(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	repo := repository.NewPostgresSurveyRepo(db)

	// 1) 直接 DB 写入 1 个 survey
	require.NoError(t, runSQL(ctx, mustDSN(t, ctx, pgC),
		`INSERT INTO emotion_echo_assessment.surveys (code, title, description) VALUES ('PHQ-9', 'PHQ Depression', '9 items')`))

	// 2) GetByCode 命中
	s, err := repo.GetByCode(ctx, "PHQ-9")
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, "PHQ-9", s.Code)
	require.Equal(t, "PHQ Depression", s.Title)

	// 3) GetByCode 未命中 -> nil, nil
	miss, err := repo.GetByCode(ctx, "NONE-EXIST")
	require.NoError(t, err)
	require.Nil(t, miss)

	// 4) List 至少 1 条
	list, err := repo.List(ctx, 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(list), 1)

	// 5) Ping
	require.NoError(t, repo.Ping(ctx))

	// 6) GetByID 命中
	got, err := repo.GetByID(ctx, s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// TestAssessment_Integration_PostgresSurveyResultCRUD SurveyResult 用户隔离 + 计算
func TestAssessment_Integration_PostgresSurveyResultCRUD(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	repo := repository.NewPostgresSurveyRepo(db)

	// 准备 1 个 PHQ-9 survey
	require.NoError(t, runSQL(ctx, mustDSN(t, ctx, pgC),
		`INSERT INTO emotion_echo_assessment.surveys (code, title) VALUES ('PHQ-9', 'PHQ')`))
	survey, err := repo.GetByCode(ctx, "PHQ-9")
	require.NoError(t, err)
	require.NotNil(t, survey)

	// 跑真实评分（PHQ-9 total=5 mild）
	ans := make(map[string]int)
	for i := 1; i <= 9; i++ {
		ans["q"+itoa(i)] = 1
	}
	result, err := scoring.Score(survey, ans)
	require.NoError(t, err)
	require.Equal(t, "mild", result.RiskLevel)
	require.Equal(t, float64(9), result.TotalScore)

	// 保存 SurveyResult
	userID := int64(7)
	sr := &model.SurveyResult{
		UserID:       userID,
		SurveyID:     survey.ID,
		Answers:      model.JSONMap{"q1": 1},
		TotalScore:   result.TotalScore,
		FactorScores: model.JSONMap{"cognitive": result.TotalScore},
		RiskLevel:    result.RiskLevel,
		DurationSec:  90,
	}
	require.NoError(t, repo.SaveResult(ctx, sr))
	require.NotZero(t, sr.ID)

	// 跨用户隔离：A 用户拿不到 B 用户结果
	got, err := repo.GetResult(ctx, sr.ID, 999) // 不是 user 7
	require.NoError(t, err)
	require.Nil(t, got)

	// 同用户
	got2, err := repo.GetResult(ctx, sr.ID, userID)
	require.NoError(t, err)
	require.NotNil(t, got2)
	require.Equal(t, "mild", got2.RiskLevel)
	require.InDelta(t, float64(9), got2.TotalScore, 0.01)

	// ListResultsByUser
	listed, err := repo.ListResultsByUser(ctx, userID, 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listed), 1)
}

// TestAssessment_Integration_PostgresDown 停容器后 repo.Ping 不 panic
func TestAssessment_Integration_PostgresDown(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	repo := repository.NewPostgresSurveyRepo(db)

	// 起阶段
	require.NoError(t, repo.Ping(ctx))

	// 停
	require.NoError(t, pgC.Terminate(ctx))

	// 再 ping 不 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Ping should not panic on db down: %v", r)
		}
	}()
	err := repo.Ping(ctx)
	// err 非 nil 也正常（容器关闭）
	_ = err
}

// mustDSN 从容器拿 DSN
func mustDSN(t *testing.T, ctx context.Context, pgC *pgcontainer.PostgresContainer) string {
	t.Helper()
	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

// itoa：测试用 1..9
func itoa(i int) string {
	if i < 0 {
		i = -i
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if len(digits) == 0 {
		return "0"
	}
	return string(digits)
}
