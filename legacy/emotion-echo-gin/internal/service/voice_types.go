package service

// VoiceProcessResult 语音处理结果
type VoiceProcessResult struct {
	MessageID    string
	Transcript   string
	Emotion      string
	EmotionLabel string
	AudioURL     string
	Duration     int
}
