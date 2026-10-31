package serviceimpl

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

var vnPhone = regexp.MustCompile(`^\+84[1-9][0-9]{8,9}$`)

type PhoneIdentityService struct {
	repo   repository.VerificationRepository
	secret []byte
	demo   bool
}

func NewPhoneIdentityService(repo repository.VerificationRepository, secret string, demo bool) (*PhoneIdentityService, error) {
	if len(secret) < 32 {
		return nil, errors.New("PHONE_HMAC_SECRET must contain at least 32 bytes")
	}
	return &PhoneIdentityService{repo: repo, secret: []byte(secret), demo: demo}, nil
}

func normalizePhone(raw string) (string, error) {
	if len(raw) > 40 {
		return "", apperror.ErrValidation.WithMessage("Số điện thoại không hợp lệ")
	}
	phone := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(raw))
	if strings.HasPrefix(phone, "0") {
		phone = "+84" + phone[1:]
	} else if strings.HasPrefix(phone, "84") {
		phone = "+" + phone
	}
	if !vnPhone.MatchString(phone) {
		return "", apperror.ErrValidation.WithMessage("Nhập số điện thoại Việt Nam hợp lệ")
	}
	return phone, nil
}

func (s *PhoneIdentityService) Resolve(ctx context.Context, raw string) (*entity.CheckoutContact, error) {
	phone, err := normalizePhone(raw)
	if err != nil {
		return nil, err
	}
	contact := &entity.CheckoutContact{PhoneE164: phone, BatchID: entity.DemoBatchID, KeySource: "hmac"}
	if s.demo {
		key, err := s.repo.DemoPhoneKey(ctx, phone)
		if err != nil {
			return nil, err
		}
		if key != "" {
			contact.PhoneKey, contact.KeySource = key, "demo_mapping"
			return contact, nil
		}
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(phone))
	contact.PhoneKey = "hmac:v1:" + hex.EncodeToString(mac.Sum(nil))
	return contact, nil
}
