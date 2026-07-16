// Stage 23: AI 多模态 endpoint handler

package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"emotion-echo-ai-svc/internal/aiclient"
	"emotion-echo-ai-svc/internal/logic"
	"emotion-echo-ai-svc/internal/svc"

	"github.com/gin-gonic/gin"
)

// MultiModalAnalyzeHandler  POST /api/v1/multimodal/analyze
//
// multipart/form-data:
//   - kind:     "image" | "audio" | "text"
//   - file:     binary (image/audio)
//   - filename: file name (optional)
//   - text:     text content (used when kind=text, or as auxiliary)
func MultiModalAnalyzeHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		kind := c.PostForm("kind")
		text := c.PostForm("text")

		var (
			fileBytes []byte
			filename  string
		)
		fh, err := c.FormFile("file")
		if err == nil && fh != nil {
			f, err := fh.Open()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read uploaded file"})
				return
			}
			defer f.Close()
			fileBytes, err = io.ReadAll(f)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "read file: " + err.Error()})
				return
			}
			filename = fh.Filename
		}

		// kind=text 也允许直接走纯文本（不需要 file）
		if kind == "text" && fileBytes == nil {
			fileBytes = nil
		}
		// 其它 kind 必须要有 file
		if kind != "text" && len(fileBytes) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file is required for kind=" + kind})
			return
		}

		resp, err := logic.NewMultiModalAnalyzeLogic(svcCtx).Analyze(c.Request.Context(), kind, fileBytes, filename, text)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// SynthesizeSpeechHandler  POST /api/v1/tts/synthesize
//
// request JSON: { "text": "...", "language": "zh-cn", "speed": 0.75 }
func SynthesizeSpeechHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	type req struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Speed    float64 `json:"speed"`
	}
	return func(c *gin.Context) {
		var r req
		if err := c.ShouldBindJSON(&r); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewSynthesizeSpeechLogic(svcCtx).Synthesize(
			c.Request.Context(), r.Text, r.Language, r.Speed,
		)
		if err != nil {
			// 任何"上游不可用"错误都归为 503（feature disabled / upstream down），
			// 而不是 500（这是 ai-svc 自己的 bug）。
			msg := err.Error()
			if errors.Is(err, logic.ErrMultiModalNotInit) ||
				errors.Is(err, logic.ErrXTTSUnavailable) ||
				strings.Contains(msg, "call XTTS") ||
				strings.Contains(msg, "XTTS_BASE_URL") ||
				errors.Is(err, aiclient.ErrNotConfigured) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": msg})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// AIHealthHandler  GET /api/v1/ai/health
//
// 探测 FER / SenseVoice / XTTS 集群健康
func AIHealthHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewAIHealthLogic(svcCtx).Health(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		status := http.StatusOK
		if !resp.All {
			// 部分 AI 服务不可用 → 200 + body 标记 unhealthy（避免 K8s liveness 把整个 svc kill）
			// 真正的 readiness 看 /api/v1/ai/health 字段
		}
		c.JSON(status, resp)
	}
}
