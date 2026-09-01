// Package repository — JSONB 归一化工具
//
// Stage 34: Postgres 不接受空串作为 JSONB 字面量（"invalid input syntax for type json"）。
// GORM 把 Go string 零值 `""` 直接 insert 会报错。
//
// 在 repo 层 Create/Upsert 前统一归一化为 `'{}'`，避免 model 层污染。
package repository

import "emotion-echo-ai-svc/internal/model"

// normalizeJSONB 把 model 里的 JSONB 字段空串归一化为 "{}"。
//
// 设计：使用 interface 避免 import 三个 model；reflect 拉字段值。
//
// 覆盖字段：
//   - FaceEmotionResult.EmotionScores
//   - FaceEmotionResult.RawResponse
//   - VoiceEmotionResult.EmotionScores
//   - VoiceEmotionResult.RawResponse
//   - FusedEmotion.ModalityContrib
func normalizeJSONB(v interface{}) {
	switch m := v.(type) {
	case *model.FaceEmotionResult:
		m.EmotionScores = orEmptyJSON(m.EmotionScores)
		m.RawResponse = orEmptyJSON(m.RawResponse)
	case *model.VoiceEmotionResult:
		m.EmotionScores = orEmptyJSON(m.EmotionScores)
		m.RawResponse = orEmptyJSON(m.RawResponse)
	case *model.FusedEmotion:
		m.ModalityContrib = orEmptyJSON(m.ModalityContrib)
	}
}

// orEmptyJSON 返回 "{}" 当输入为空。
func orEmptyJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}