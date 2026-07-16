package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/workflow/assessment"
)

// BatchDailyAssessment 批量每日评估（使用 goroutine pool 并行）
func BatchDailyAssessment(ctx context.Context, assessmentRepo *repository.MentalHealthRepository, workflow *assessment.Workflow) error {
	// 获取昨天的时间范围
	now := time.Now()
	startTime := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	endTime := startTime.Add(24 * time.Hour)

	// 获取需要评估的用户列表
	userIDs, err := assessmentRepo.GetUsersNeedingAssessment(ctx, startTime, endTime, 1000)
	if err != nil {
		return err
	}

	if len(userIDs) == 0 {
		return nil
	}

	// 使用 goroutine pool 并行评估（限制并发数防止 LLM API 限流）
	const maxConcurrency = 10
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failedCount int

	for _, userID := range userIDs {
		// 检查今天是否已评估
		hasToday, err := assessmentRepo.HasAssessmentToday(ctx, userID, "daily")
		if err != nil {
			continue
		}
		if hasToday {
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(uid int64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// 执行评估
			result, err := workflow.Execute(ctx, uid, "daily", 1)
			if err != nil {
				mu.Lock()
				failedCount++
				mu.Unlock()
				return
			}

			// 保存结果
			if err := assessmentRepo.Create(ctx, result); err != nil {
				mu.Lock()
				failedCount++
				mu.Unlock()
			}
		}(userID)
	}

	wg.Wait()

	if failedCount > 0 {
		return fmt.Errorf("batch assessment completed with %d failures", failedCount)
	}

	return nil
}
