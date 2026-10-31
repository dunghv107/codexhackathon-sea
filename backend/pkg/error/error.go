package apperror

import (
	"fmt"
	"net/http"
)

// AppError represents an application error with code, message, and HTTP status
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Err        error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WithMessage returns a new AppError with an updated message
func (e *AppError) WithMessage(message string) *AppError {
	return &AppError{
		Code:       e.Code,
		Message:    message,
		HTTPStatus: e.HTTPStatus,
		Err:        e.Err,
	}
}

// WithError wraps an underlying error
func (e *AppError) WithError(err error) *AppError {
	return &AppError{
		Code:       e.Code,
		Message:    e.Message,
		HTTPStatus: e.HTTPStatus,
		Err:        err,
	}
}

// Common errors
var (
	// 400 Bad Request
	ErrBadRequest = &AppError{
		Code:       "BAD_REQUEST",
		Message:    "Yêu cầu không hợp lệ",
		HTTPStatus: http.StatusBadRequest,
	}

	ErrValidation = &AppError{
		Code:       "VALIDATION_ERROR",
		Message:    "Dữ liệu không hợp lệ",
		HTTPStatus: http.StatusBadRequest,
	}

	// 401 Unauthorized
	ErrUnauthorized = &AppError{
		Code:       "UNAUTHORIZED",
		Message:    "Chưa xác thực",
		HTTPStatus: http.StatusUnauthorized,
	}

	ErrInvalidCredentials = &AppError{
		Code:       "INVALID_CREDENTIALS",
		Message:    "Thông tin đăng nhập không chính xác",
		HTTPStatus: http.StatusUnauthorized,
	}

	ErrTokenExpired = &AppError{
		Code:       "TOKEN_EXPIRED",
		Message:    "Token đã hết hạn",
		HTTPStatus: http.StatusUnauthorized,
	}

	ErrRefreshTokenInvalid = &AppError{
		Code:       "REFRESH_TOKEN_INVALID",
		Message:    "Phiên đăng nhập không hợp lệ hoặc đã hết hạn",
		HTTPStatus: http.StatusUnauthorized,
	}

	// 403 Forbidden
	ErrForbidden = &AppError{
		Code:       "FORBIDDEN",
		Message:    "Không có quyền truy cập",
		HTTPStatus: http.StatusForbidden,
	}

	// 404 Not Found
	ErrNotFound = &AppError{
		Code:       "NOT_FOUND",
		Message:    "Không tìm thấy tài nguyên",
		HTTPStatus: http.StatusNotFound,
	}

	ErrUserNotFound = &AppError{
		Code:       "USER_NOT_FOUND",
		Message:    "Không tìm thấy người dùng",
		HTTPStatus: http.StatusNotFound,
	}

	// 409 Conflict
	ErrConflict = &AppError{
		Code:       "CONFLICT",
		Message:    "Dữ liệu đã tồn tại",
		HTTPStatus: http.StatusConflict,
	}

	ErrUsernameExists = &AppError{
		Code:       "USERNAME_EXISTS",
		Message:    "Tên đăng nhập đã tồn tại",
		HTTPStatus: http.StatusConflict,
	}

	ErrEmailExists = &AppError{
		Code:       "EMAIL_EXISTS",
		Message:    "Email đã tồn tại",
		HTTPStatus: http.StatusConflict,
	}

	// 429 Too Many Requests
	ErrTooManyRequests = &AppError{
		Code:       "TOO_MANY_REQUESTS",
		Message:    "Quá nhiều yêu cầu, vui lòng thử lại sau",
		HTTPStatus: http.StatusTooManyRequests,
	}

	// 500 Internal Server Error
	ErrInternalServerError = &AppError{
		Code:       "INTERNAL_ERROR",
		Message:    "Đã xảy ra lỗi máy chủ nội bộ",
		HTTPStatus: http.StatusInternalServerError,
	}
)
