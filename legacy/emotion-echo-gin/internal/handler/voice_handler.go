package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

type VoiceHandler struct {
	voiceService *service.VoiceService
}

func NewVoiceHandler(voiceService *service.VoiceService) *VoiceHandler {
	return &VoiceHandler{
		voiceService: voiceService,
	}
}

func (h *VoiceHandler) Upload(c *gin.Context) {
	userID := c.GetInt64("userId")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conversationID := c.PostForm("conversationId")

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to get audio file: " + err.Error()})
		return
	}

	result, err := h.voiceService.ProcessVoiceMessage(
		c.Request.Context(),
		userID,
		conversationID,
		file,
		nil,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response.Success(c, gin.H{
		"messageId":    result.MessageID,
		"audioUrl":     result.AudioURL,
		"transcript":   result.Transcript,
		"emotion":      result.Emotion,
		"emotionLabel": result.EmotionLabel,
		"duration":     result.Duration,
	})
}
