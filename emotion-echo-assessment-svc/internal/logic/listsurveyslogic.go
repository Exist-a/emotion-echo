// Code scaffolded by goctl. Safe to edit.

package logic

import (
	"context"

	"emotion-echo-assessment-svc/internal/model"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListSurveysLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSurveysLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSurveysLogic {
	return &ListSurveysLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListSurveys 返回所有 active 量表
func (l *ListSurveysLogic) ListSurveys(req *types.ListSurveysReq) (resp *types.ListSurveysResp, err error) {
	limit := 50 // 默认
	if req != nil && req.Limit > 0 {
		limit = req.Limit
	}
	surveys, err := l.svcCtx.SurveyRepo.List(l.ctx, limit)
	if err != nil {
		return nil, err
	}

	resp = &types.ListSurveysResp{
		Items: make([]types.SurveyItem, 0, len(surveys)),
		Total: len(surveys),
	}
	for _, s := range surveys {
		// 只返回 active 的（status=1）
		if s.Status != 1 {
			continue
		}
		resp.Items = append(resp.Items, types.SurveyItem{
			ID:          s.ID,
			Code:        s.Code,
			Title:       s.Title,
			Description: s.Description,
			Category:    s.Category,
			QuestionNum: countQuestions(s.Questions),
			Version:     s.Version,
		})
	}
	resp.Total = len(resp.Items)
	return resp, nil
}

// countQuestions 从 JSONMap 中提取题目数
//
// 不同量表的 questions 结构不同：
//   - PHQ-9: {"items": [...]}  → items.length
//   - GAD-7: 同上
//   - 自定义: {"q1": ..., "q2": ...}  → map 大小
func countQuestions(q model.JSONMap) int {
	if len(q) == 0 {
		return 0
	}
	// 尝试 items 数组
	if items, ok := q["items"]; ok {
		if arr, ok := items.([]any); ok {
			return len(arr)
		}
	}
	// fallback：map 大小（不算嵌套）
	return len(q)
}