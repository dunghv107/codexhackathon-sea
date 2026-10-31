package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/middleware"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
)

// ProfileHandler interface
type ProfileHandler interface {
	GetProfiles(c *gin.Context)
}

type profileHandlerImpl struct {
	profileService service.ProfileService
}

// NewProfileHandler creates a new profile handler
func NewProfileHandler(profileService service.ProfileService) ProfileHandler {
	return &profileHandlerImpl{profileService: profileService}
}

func (h *profileHandlerImpl) GetProfiles(c *gin.Context) {
	profiles, err := h.profileService.GetProfiles(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}

	var resp dto.ProfilesResponse
	if b := profiles.Buyer; b != nil {
		resp.Buyer = &dto.BuyerProfileResponse{BuyerID: b.BuyerID, DisplayName: b.DisplayName, Status: b.Status}
	}
	if s := profiles.Seller; s != nil {
		resp.Seller = &dto.SellerProfileResponse{SellerID: s.SellerID, DisplayName: s.DisplayName, Status: s.Status}
	}
	response.OK(c, resp, "")
}
