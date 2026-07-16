package text

import (
	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/workflow/graph"
)

const (
	KeyText             = "text"
	KeyMessages         = "messages"
	KeyEmotion          = "emotion"
	KeyConfidence       = "confidence"
	KeyAllScores       = "all_scores"
	KeySystemPrompt     = "system_prompt"
	KeyKeywords         = "keywords"
	KeySummary          = "summary"
	KeySuggestion       = "suggestion"
	KeyIntent           = "intent"
	KeyIntentConfidence = "intent_confidence"
	KeyAudioContext     = "audio_context"
	KeyIsVoiceMessage   = "is_voice_message"
)

type TextState struct {
	graph.MemoryState
}

func NewTextState() *TextState {
	return &TextState{
		MemoryState: *graph.NewMemoryState(),
	}
}

func (s *TextState) SetText(text string) {
	s.Set(KeyText, text)
}

func (s *TextState) GetText() string {
	return s.GetString(KeyText)
}

func (s *TextState) SetMessages(messages []*models.Message) {
	s.Set(KeyMessages, messages)
}

func (s *TextState) GetMessages() []*models.Message {
	val, _ := s.Get(KeyMessages)
	if messages, ok := val.([]*models.Message); ok {
		return messages
	}
	return nil
}

func (s *TextState) SetEmotion(emotion string) {
	s.Set(KeyEmotion, emotion)
}

func (s *TextState) GetEmotion() string {
	return s.GetString(KeyEmotion)
}

func (s *TextState) SetConfidence(confidence float64) {
	s.Set(KeyConfidence, confidence)
}

func (s *TextState) GetConfidence() float64 {
	return s.GetFloat(KeyConfidence)
}

func (s *TextState) SetSystemPrompt(prompt string) {
	s.Set(KeySystemPrompt, prompt)
}

func (s *TextState) GetSystemPrompt() string {
	return s.GetString(KeySystemPrompt)
}

func (s *TextState) SetKeywords(keywords []string) {
	s.Set(KeyKeywords, keywords)
}

func (s *TextState) GetKeywords() []string {
	return s.GetStringSlice(KeyKeywords)
}

func (s *TextState) SetSummary(summary string) {
	s.Set(KeySummary, summary)
}

func (s *TextState) GetSummary() string {
	return s.GetString(KeySummary)
}

func (s *TextState) SetSuggestion(suggestion string) {
	s.Set(KeySuggestion, suggestion)
}

func (s *TextState) GetSuggestion() string {
	return s.GetString(KeySuggestion)
}

func (s *TextState) SetIntent(intent string) {
	s.Set(KeyIntent, intent)
}

func (s *TextState) GetIntent() string {
	return s.GetString(KeyIntent)
}

func (s *TextState) SetIntentConfidence(confidence float64) {
	s.Set(KeyIntentConfidence, confidence)
}

func (s *TextState) GetIntentConfidence() float64 {
	return s.GetFloat(KeyIntentConfidence)
}

func (s *TextState) BuildRawContent() string {
	messages := s.GetMessages()
	if messages == nil {
		return s.GetText()
	}
	var content string
	for _, msg := range messages {
		content += msg.Sender + ": " + msg.Content + "\n"
	}
	return content
}

func (s *TextState) GetEmotionFromState() string {
	val, _ := s.Get(KeyEmotion)
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

func (s *TextState) GetConfidenceFromState() float64 {
	val, _ := s.Get(KeyConfidence)
	if f, ok := val.(float64); ok {
		return f
	}
	return 0
}

func (s *TextState) SetAllScores(scores map[string]float64) {
	s.Set(KeyAllScores, scores)
}

func (s *TextState) GetAllScores() map[string]float64 {
	val, _ := s.Get(KeyAllScores)
	if scores, ok := val.(map[string]float64); ok {
		return scores
	}
	return nil
}

func (s *TextState) SetAudioContext(transcript, emotion string) {
	s.Set(KeyAudioContext, map[string]string{
		"transcript": transcript,
		"emotion":    emotion,
	})
	s.Set(KeyIsVoiceMessage, true)
}

func (s *TextState) IsVoiceMessage() bool {
	val, _ := s.Get(KeyIsVoiceMessage)
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

func (s *TextState) GetAudioContext() (transcript, emotion string) {
	val, _ := s.Get(KeyAudioContext)
	if m, ok := val.(map[string]string); ok {
		return m["transcript"], m["emotion"]
	}
	return "", ""
}
