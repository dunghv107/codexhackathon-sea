package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/middleware"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
	"io"
	"net/http"
	"strconv"
)

type RiskDataHandler struct{ service service.RiskDataService }

func NewRiskDataHandler(service service.RiskDataService) *RiskDataHandler {
	return &RiskDataHandler{service: service}
}

func (h *RiskDataHandler) Record(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 850_000)
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	var input service.RiskEvidenceInput
	if err := d.Decode(&input); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Bản ghi không hợp lệ hoặc quá lớn"))
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Chỉ chấp nhận một bản ghi"))
		return
	}
	if err := h.service.Record(c.Request.Context(), middleware.GetUserID(c), input); err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.Created(c, gin.H{"record_id": input.RecordID}, "Đã ghi nhận bằng chứng; áp dụng cho các lần mua sau thời điểm hệ thống nhận dữ liệu")
}

func (h *RiskDataHandler) ListPlatform(c *gin.Context) {
	page, e1 := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, e2 := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if e1 != nil || e2 != nil || page < 1 || page > 100000 || limit < 1 || limit > 100 {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Phân trang không hợp lệ"))
		return
	}
	items, total, err := h.service.ListPlatform(c.Request.Context(), c.DefaultQuery("status", "fired"), (page-1)*limit, limit)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, dto.ListResponse[entity.PlatformRiskObservation]{Items: items, Total: total, Page: page, Limit: limit, TotalPages: int((total + int64(limit) - 1) / int64(limit))}, "")
}
