package persistence

import (
	"errors"

	"gorm.io/gorm"

	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

func isNotFoundError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func wrapFindError(err error, entityName string) error {
	if isNotFoundError(err) {
		return apperror.ErrNotFound.WithMessage("Không tìm thấy " + entityName).WithError(err)
	}
	return apperror.ErrInternalServerError.WithMessage("Không thể truy vấn " + entityName).WithError(err)
}

func wrapCreateError(err error, entityName string) error {
	if isDuplicateKeyError(err) {
		return apperror.ErrConflict.WithMessage(entityName + " đã tồn tại").WithError(err)
	}
	return apperror.ErrInternalServerError.WithMessage("Không thể tạo " + entityName).WithError(err)
}

func wrapUpdateError(err error, entityName string) error {
	if isDuplicateKeyError(err) {
		return apperror.ErrConflict.WithMessage(entityName + " đã tồn tại").WithError(err)
	}
	return apperror.ErrInternalServerError.WithMessage("Không thể cập nhật " + entityName).WithError(err)
}

func wrapDeleteError(err error, entityName string) error {
	return apperror.ErrInternalServerError.WithMessage("Không thể xóa " + entityName).WithError(err)
}

func wrapListError(err error, entityName string) error {
	return apperror.ErrInternalServerError.WithMessage("Không thể lấy danh sách " + entityName).WithError(err)
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "duplicate key") ||
		contains(errStr, "Duplicate entry") ||
		contains(errStr, "UNIQUE constraint failed") ||
		contains(errStr, "violates unique constraint")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsRune(s, substr))
}

func containsRune(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
