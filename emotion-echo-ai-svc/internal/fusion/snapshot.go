// Package fusion — Stage 34 · PR-10 GREEN
//
// ModalitySnapshot 是 FusionWorker 拼装后的输入结构体，
// 喂给 LateFuser 和 LLMFuser。
//
// 设计原则：
//   - 三路都是可选（用户可能只上传文字 / 只上传图片 / 只上传语音）
//   - Emotion 字段是各模态的主标签（happy/sad/angry/...）
//   - Confidence 是模型自报的可信度（0~1）
//   - Sentiment 是 -1~1 的情感极性（text/voice 通常有，face 通常无 → 0）
//   - Source 是来源标记（如 "text"/"fer:fer"/"sensevoice:sensevoice-small"）
package fusion

// ModalityScore 单模态情绪识别结果。
type ModalityScore struct {
	Emotion    string  // 主标签
	Confidence float64 // 模型自报可信度 [0, 1]
	Sentiment  float64 // 情感极性 [-1, 1]
	Source     string  // 来源标记（"text"/"fer:fer"/"sensevoice:..."）
	Scores     string  // 完整分数表 JSON（可空）
}

// ModalitySnapshot 同一消息的三路情绪分数。
type ModalitySnapshot struct {
	Text  *ModalityScore
	Face  *ModalityScore
	Voice *ModalityScore
}

// AvailableModalities 返回实际有数据的模态名。
func (s ModalitySnapshot) AvailableModalities() []string {
	out := make([]string, 0, 3)
	if s.Text != nil {
		out = append(out, "text")
	}
	if s.Face != nil {
		out = append(out, "face")
	}
	if s.Voice != nil {
		out = append(out, "voice")
	}
	return out
}

// IsEmpty 是否零模态。
func (s ModalitySnapshot) IsEmpty() bool {
	return s.Text == nil && s.Face == nil && s.Voice == nil
}
