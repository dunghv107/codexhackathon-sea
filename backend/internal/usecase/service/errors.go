package service

import (
	"net/http"

	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// Marketplace error codes from docs/API_CONTRACT.md
var (
	ErrNoBuyerProfile = &apperror.AppError{
		Code:       "NO_BUYER_PROFILE",
		Message:    "Tài khoản chưa có hồ sơ người mua",
		HTTPStatus: http.StatusForbidden,
	}

	ErrNoSellerProfile = &apperror.AppError{
		Code:       "NO_SELLER_PROFILE",
		Message:    "Tài khoản chưa có hồ sơ người bán",
		HTTPStatus: http.StatusForbidden,
	}

	ErrAccountSuspended = &apperror.AppError{
		Code:       "ACCOUNT_SUSPENDED",
		Message:    "Tài khoản đang bị tạm khóa",
		HTTPStatus: http.StatusForbidden,
	}

	ErrMixedCurrency = &apperror.AppError{
		Code:       "MIXED_CURRENCY",
		Message:    "Các sản phẩm phải cùng loại tiền tệ",
		HTTPStatus: http.StatusBadRequest,
	}

	ErrOutOfStock = &apperror.AppError{
		Code:       "OUT_OF_STOCK",
		Message:    "Sản phẩm không đủ hàng",
		HTTPStatus: http.StatusConflict,
	}

	ErrNotAssessed = &apperror.AppError{
		Code:       "NOT_ASSESSED",
		Message:    "Đơn hàng chưa được đánh giá rủi ro",
		HTTPStatus: http.StatusConflict,
	}

	ErrStaleAssessment = &apperror.AppError{
		Code:       "STALE_ASSESSMENT",
		Message:    "Đánh giá rủi ro đã cũ, vui lòng tải lại đơn hàng",
		HTTPStatus: http.StatusConflict,
	}

	ErrInvalidState = &apperror.AppError{
		Code:       "INVALID_STATE",
		Message:    "Trạng thái đơn hàng không cho phép thao tác này",
		HTTPStatus: http.StatusConflict,
	}

	ErrInvalidTransition = &apperror.AppError{
		Code:       "INVALID_TRANSITION",
		Message:    "Không thể chuyển sang trạng thái này",
		HTTPStatus: http.StatusConflict,
	}

	ErrPaymentNotReady = &apperror.AppError{
		Code:       "PAYMENT_NOT_READY",
		Message:    "Đơn chưa thanh toán thành công",
		HTTPStatus: http.StatusConflict,
	}
)
