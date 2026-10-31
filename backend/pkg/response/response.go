package response

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/thienel/tlog"
	"go.uber.org/zap"

	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// APIResponse is the standard API response format
type APIResponse[T any] struct {
	IsSuccess bool   `json:"is_success"`
	Data      T      `json:"data"`
	Message   string `json:"message,omitempty"`
	Error     *Error `json:"error,omitempty"`
}

// Error represents an error in the API response
type Error struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Fields  []FieldError `json:"fields,omitempty"`
}

// FieldError represents a validation error for a specific field
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// OK sends a 200 OK response
func OK[T any](c *gin.Context, data T, message string) {
	c.JSON(http.StatusOK, APIResponse[T]{
		IsSuccess: true,
		Data:      data,
		Message:   message,
	})
}

// Created sends a 201 Created response
func Created[T any](c *gin.Context, data T, message string) {
	c.JSON(http.StatusCreated, APIResponse[T]{
		IsSuccess: true,
		Data:      data,
		Message:   message,
	})
}

// NoContent sends a 204 No Content response
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// ValidationError sends a 400 Bad Request with validation errors
func ValidationError(c *gin.Context, fields []FieldError) {
	c.JSON(http.StatusBadRequest, APIResponse[any]{
		IsSuccess: false,
		Error: &Error{
			Code:    "VALIDATION_ERROR",
			Message: "Dữ liệu không hợp lệ",
			Fields:  fields,
		},
	})
}

// getStackTrace captures the stack trace for debugging
func getStackTrace(skip int) string {
	const maxStackLen = 2048
	buf := make([]byte, maxStackLen)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// getRequestContext extracts useful context from the request for logging
func getRequestContext(c *gin.Context) []zap.Field {
	fields := []zap.Field{
		zap.String("path", c.Request.URL.Path),
		zap.String("method", c.Request.Method),
		zap.String("client_ip", c.ClientIP()),
	}

	if requestID := c.GetHeader("X-Request-ID"); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}

	if userID, exists := c.Get("user_id"); exists {
		fields = append(fields, zap.Any("user_id", userID))
	}

	return fields
}

// WriteErrorResponse writes an error response based on the error type
func WriteErrorResponse(c *gin.Context, err error) {
	if appErr, ok := err.(*apperror.AppError); ok {
		logFields := getRequestContext(c)
		logFields = append(logFields,
			zap.String("error_code", appErr.Code),
			zap.String("error_message", appErr.Message),
			zap.Int("http_status", appErr.HTTPStatus),
		)

		if appErr.Err != nil {
			logFields = append(logFields, zap.Error(appErr.Err))
		}

		if appErr.HTTPStatus >= 500 {
			logFields = append(logFields, zap.String("stack_trace", getStackTrace(2)))
			tlog.Error("Server error", logFields...)
		} else if appErr.HTTPStatus >= 400 {
			tlog.Warn("Client error", logFields...)
		}

		c.JSON(appErr.HTTPStatus, APIResponse[any]{
			IsSuccess: false,
			Error: &Error{
				Code:    appErr.Code,
				Message: appErr.Message,
			},
		})
		return
	}

	// Log unexpected errors
	logFields := getRequestContext(c)
	logFields = append(logFields,
		zap.Error(err),
		zap.String("stack_trace", getStackTrace(2)),
	)
	tlog.Error("Unexpected error", logFields...)

	c.JSON(http.StatusInternalServerError, APIResponse[any]{
		IsSuccess: false,
		Error: &Error{
			Code:    "INTERNAL_ERROR",
			Message: "Đã xảy ra lỗi máy chủ nội bộ",
		},
	})
}
