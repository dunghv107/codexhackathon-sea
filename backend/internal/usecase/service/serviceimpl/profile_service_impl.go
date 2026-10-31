package serviceimpl

import (
	"context"
	"fmt"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
)

type profileServiceImpl struct {
	profileRepo repository.ProfileRepository
}

// NewProfileService creates a new profile service
func NewProfileService(profileRepo repository.ProfileRepository) service.ProfileService {
	return &profileServiceImpl{profileRepo: profileRepo}
}

// authSubject links a template login to buyers/sellers.auth_subject.
func authSubject(userID uint) string {
	return fmt.Sprintf("local:%d", userID)
}

func (s *profileServiceImpl) GetProfiles(ctx context.Context, userID uint) (*service.Profiles, error) {
	buyer, err := s.ResolveBuyer(ctx, userID)
	if err != nil && err != service.ErrNoBuyerProfile {
		return nil, err
	}
	seller, err := s.ResolveSeller(ctx, userID)
	if err != nil && err != service.ErrNoSellerProfile {
		return nil, err
	}
	return &service.Profiles{Buyer: buyer, Seller: seller}, nil
}

func (s *profileServiceImpl) ResolveBuyer(ctx context.Context, userID uint) (*entity.Buyer, error) {
	buyer, err := s.profileRepo.FindBuyerByAuthSubject(ctx, authSubject(userID))
	if isNotFound(err) {
		return nil, service.ErrNoBuyerProfile
	}
	return buyer, err
}

func (s *profileServiceImpl) ResolveSeller(ctx context.Context, userID uint) (*entity.Seller, error) {
	seller, err := s.profileRepo.FindSellerByAuthSubject(ctx, authSubject(userID))
	if isNotFound(err) {
		return nil, service.ErrNoSellerProfile
	}
	return seller, err
}
