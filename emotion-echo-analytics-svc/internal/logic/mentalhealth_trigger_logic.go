// Package logic — mentalhealth_trigger_logic.go
//
// Stage 30-A Round 3 part 2 GREEN: POST /api/v1/mental-health/trigger.
//
// 异步触发契约（per docs/stage-30-A §三.2）：
//   - Logic 校验 userID + assessmentType
//   - 提交到 TriggerQueue.Submit(...)
//   - 立即返回 task_id + status="accepted"
//   - queue 满 → ErrQueueFull（backpressure，handler 返 503）
//   - queue 已关 → ErrQueueClosed
package logic

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/trigger"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// MentalHealthTriggerLogic 处理异步触发评估
type MentalHealthTriggerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMentalHealthTriggerLogic 构造
func NewMentalHealthTriggerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MentalHealthTriggerLogic {
	return &MentalHealthTriggerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// TriggerAssessment 校验 + 提交 queue + 立即返回 task_id
//
//   - req.UserID <= 0 → validation error
//   - req.AssessmentType 不在 {daily, weekly, comprehensive} → validation error
//   - svcCtx.TriggerQueue == nil → internal error（区别于 backpressure）
//   - queue.Submit 返 ErrQueueFull → backpressure（handler 应返 503）
//   - queue.Submit 返 ErrQueueClosed → closed（handler 应返 503）
//   - 其他错误 → propagate
func (l *MentalHealthTriggerLogic) TriggerAssessment(req *types.TriggerMentalHealthReq) (*types.TriggerMentalHealthResp, error) {
	if req == nil {
		return nil, errors.New("validation: request is nil")
	}
	if req.UserID <= 0 {
		return nil, errors.New("validation: user id must be positive")
	}
	if !repository.IsValidAssessmentType(req.AssessmentType) {
		return nil, fmt.Errorf("validation: invalid assessment_type %q (must be daily|weekly|comprehensive)",
			req.AssessmentType)
	}

	if l.svcCtx.TriggerQueue == nil {
		return nil, errors.New("internal: trigger queue not configured")
	}

	taskID := uuid.NewString()
	err := l.svcCtx.TriggerQueue.Submit(trigger.Request{
		UserID:         req.UserID,
		AssessmentType: req.AssessmentType,
		TraceID:        taskID,
	})
	if err != nil {
		// 透传 queue error（backpressure / closed）；caller 决定 status code
		return nil, err
	}

	return &types.TriggerMentalHealthResp{
		TaskID: taskID,
		Status: "accepted",
		TraceID: taskID,
	}, nil
}