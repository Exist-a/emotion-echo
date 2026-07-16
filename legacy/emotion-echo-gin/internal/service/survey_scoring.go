package service

import (
	"fmt"

	"emotion-echo-gin/internal/models"
)

// ScoreResult 量表评分结果
type ScoreResult struct {
	RawScore      int    `json:"rawScore"`      // 原始分（所有选项得分之和）
	StandardScore int    `json:"standardScore"` // 标准分（SDS/SAS: raw × 1.25）
	Level         string `json:"level"`         // 等级：正常/轻度/中度/重度
	Suggestion    string `json:"suggestion"`    // 建议文本
}

// SurveyScorer 量表评分器接口
type SurveyScorer interface {
	// Calculate 计算量表得分
	// answers: 用户答案
	// questions: 量表题目定义（含选项分数）
	Calculate(answers []models.UserAnswer, questions []models.SurveyQuestion) (*ScoreResult, error)
}

// NewSurveyScorer 根据量表ID创建对应的评分器
func NewSurveyScorer(surveyID int) SurveyScorer {
	switch surveyID {
	case 1:
		return &SDSScorer{}
	case 2:
		return &SASScorer{}
	default:
		return &DefaultScorer{}
	}
}

// DefaultScorer 默认评分器（简单累加）
type DefaultScorer struct{}

func (s *DefaultScorer) Calculate(answers []models.UserAnswer, questions []models.SurveyQuestion) (*ScoreResult, error) {
	rawScore := 0
	for _, answer := range answers {
		for _, q := range questions {
			if q.ID == answer.QuestionID {
				for _, opt := range q.Options {
					if opt.ID == answer.OptionID {
						rawScore += opt.Score
						break
					}
				}
				break
			}
		}
	}

	return &ScoreResult{
		RawScore:      rawScore,
		StandardScore: rawScore,
		Level:         "未知",
		Suggestion:    "请根据专业医生评估结果进行参考。",
	}, nil
}

// SDSScorer SDS抑郁自评量表评分器
// - 20题，10题反向计分
// - 原始分范围：20-80
// - 标准分 = 原始分 × 1.25（范围：25-100）
// - 等级：＜50正常, 50-59轻度, 60-69中度, ≥70重度
type SDSScorer struct{}

// SDS反向计分题号（题目ID）
var sdsReverseQuestions = map[int]bool{
	2:  true,  // 我感到早晨心情最好
	5:  true,  // 我吃饭像平时一样多
	6:  true,  // 我与异性密切接触时和以往一样感到愉快
	11: true,  // 我的头脑跟平常一样清楚
	12: true,  // 我觉得经常做的事情并没有困难
	14: true,  // 我对未来抱有希望
	16: true,  // 我觉得作出决定是容易的
	17: true,  // 我感到自己是有用的和不可缺少的人
	18: true,  // 我的生活很有意义
	20: true,  // 我仍旧喜爱自己平时喜爱的东西
}

func (s *SDSScorer) Calculate(answers []models.UserAnswer, questions []models.SurveyQuestion) (*ScoreResult, error) {
	rawScore := 0

	for _, answer := range answers {
		for _, q := range questions {
			if q.ID == answer.QuestionID {
				for _, opt := range q.Options {
					if opt.ID == answer.OptionID {
						score := opt.Score
						// 动态反向计分：如果题目在反向列表中，反转分数
						// 反向计分：1→4, 2→3, 3→2, 4→1（即 5 - score）
						if sdsReverseQuestions[q.ID] {
							score = 5 - score
						}
						rawScore += score
						break
					}
				}
				break
			}
		}
	}

	// 计算标准分
	standardScore := int(float64(rawScore) * 1.25)

	// 判定等级
	level, suggestion := s.calculateLevel(standardScore)

	return &ScoreResult{
		RawScore:      rawScore,
		StandardScore: standardScore,
		Level:         level,
		Suggestion:    suggestion,
	}, nil
}

func (s *SDSScorer) calculateLevel(standardScore int) (string, string) {
	switch {
	case standardScore < 50:
		return "正常", "您的情绪状态良好，请继续保持积极的生活态度。"
	case standardScore < 60:
		return "轻度抑郁", "您有轻度抑郁症状，建议关注自己的情绪变化，适当放松心情。"
	case standardScore < 70:
		return "中度抑郁", "您有中度抑郁症状，建议寻求专业心理咨询，多与朋友家人交流。"
	default:
		return "重度抑郁", "您有重度抑郁症状，强烈建议尽快寻求专业心理医生帮助。"
	}
}

// SASScorer SAS焦虑自评量表评分器
// - 20题，全部正向计分（但Q19为反向题）
// - 原始分范围：20-80
// - 标准分 = 原始分 × 1.25（范围：25-100）
// - 等级：＜50正常, 50-59轻度, 60-69中度, ≥70重度
type SASScorer struct{}

// SAS反向计分题号
var sasReverseQuestions = map[int]bool{
	19: true, // 我容易入睡并且一夜睡得很好
}

func (s *SASScorer) Calculate(answers []models.UserAnswer, questions []models.SurveyQuestion) (*ScoreResult, error) {
	rawScore := 0

	for _, answer := range answers {
		for _, q := range questions {
			if q.ID == answer.QuestionID {
				for _, opt := range q.Options {
					if opt.ID == answer.OptionID {
						score := opt.Score
						// 动态反向计分：如果题目在反向列表中，反转分数
						if sasReverseQuestions[q.ID] {
							score = 5 - score
						}
						rawScore += score
						break
					}
				}
				break
			}
		}
	}

	// 计算标准分
	standardScore := int(float64(rawScore) * 1.25)

	// 判定等级（SAS与SDS使用相同阈值）
	level, suggestion := s.calculateLevel(standardScore)

	return &ScoreResult{
		RawScore:      rawScore,
		StandardScore: standardScore,
		Level:         level,
		Suggestion:    suggestion,
	}, nil
}

func (s *SASScorer) calculateLevel(standardScore int) (string, string) {
	switch {
	case standardScore < 50:
		return "正常", "您的焦虑水平正常，请继续保持良好的心态。"
	case standardScore < 60:
		return "轻度焦虑", "您有轻度焦虑症状，建议适当放松，进行深呼吸或冥想练习。"
	case standardScore < 70:
		return "中度焦虑", "您有中度焦虑症状，建议寻求专业心理咨询，学习焦虑管理技巧。"
	default:
		return "重度焦虑", "您有重度焦虑症状，强烈建议尽快寻求专业心理医生帮助。"
	}
}

// ValidateAnswers 验证答案完整性
func ValidateAnswers(answers []models.UserAnswer, questions []models.SurveyQuestion) error {
	if len(answers) != len(questions) {
		return fmt.Errorf("答案数量不匹配：提交了%d题，量表共%d题", len(answers), len(questions))
	}

	// 检查每道题都有答案
	questionIDs := make(map[int]bool)
	for _, q := range questions {
		questionIDs[q.ID] = true
	}

	for _, a := range answers {
		if !questionIDs[a.QuestionID] {
			return fmt.Errorf("题目ID %d 不存在", a.QuestionID)
		}
	}

	return nil
}
