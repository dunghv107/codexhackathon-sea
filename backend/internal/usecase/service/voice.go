package service

import (
	"context"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"net/http"
)

type VoiceInput struct {
	Audio    string `json:"audio" binding:"required,max=819200"`
	MimeType string `json:"mime_type" binding:"required,oneof=audio/webm audio/ogg audio/mp4 audio/wav"`
}
type VoiceResult struct {
	Transcript string `json:"transcript"`
	Intent     string `json:"intent"`
}
type VoiceClient interface {
	VoicePreview(context.Context, string, string, VoiceInput) (*VoiceResult, error)
}

var ErrVoiceUnavailable = &apperror.AppError{Code: "VOICE_UNAVAILABLE", Message: "Không nhận dạng được giọng nói. Hãy thử lại hoặc dùng phím xác nhận.", HTTPStatus: http.StatusServiceUnavailable}
