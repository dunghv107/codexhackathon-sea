package serviceimpl

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/thienel/tlog"
	"go.uber.org/zap"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/valueobject"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// currencyCode is an ISO 4217-shaped code; the frontend formats it with Intl, which throws on anything else.
var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

type sellerServiceImpl struct {
	catalogRepo repository.CatalogRepository
	orderRepo   repository.OrderRepository
	tx          repository.Transactor
}

// NewSellerService creates a new seller service
func NewSellerService(
	catalogRepo repository.CatalogRepository,
	orderRepo repository.OrderRepository,
	tx repository.Transactor,
) service.SellerService {
	return &sellerServiceImpl{catalogRepo: catalogRepo, orderRepo: orderRepo, tx: tx}
}

func (s *sellerServiceImpl) ListShops(ctx context.Context, sellerID string) ([]entity.Shop, error) {
	return s.catalogRepo.ListShopsBySeller(ctx, sellerID)
}

func (s *sellerServiceImpl) CreateShop(ctx context.Context, sellerID, displayName string) (*entity.Shop, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, apperror.ErrValidation.WithMessage("Tên shop không được để trống")
	}
	shop := &entity.Shop{ShopID: newAppID(), DisplayName: displayName, OwnerSellerID: &sellerID, OwnerActorRef: &sellerID, BatchID: entity.AppBatchID}
	shop.SourceRecordID = shop.ShopID
	if err := s.catalogRepo.CreateShop(ctx, shop); err != nil {
		return nil, err
	}

	tlog.Info("Shop created", zap.String("shop_id", shop.ShopID), zap.String("seller_id", sellerID))
	return shop, nil
}

func (s *sellerServiceImpl) GetOwnedShop(ctx context.Context, sellerID, shopID string) (*entity.Shop, error) {
	return s.catalogRepo.FindOwnedShop(ctx, shopID, sellerID)
}

func (s *sellerServiceImpl) GetDashboard(ctx context.Context, shopID string) (*service.Dashboard, error) {
	counts, err := s.orderRepo.CountByStatus(ctx, shopID)
	if err != nil {
		return nil, err
	}
	d := &service.Dashboard{
		ReviewStatus: map[string]int64{entity.ReviewPending: 0, entity.ReviewHold: 0, entity.ReviewReleased: 0},
		FulfillmentStatus: map[string]int64{
			entity.FulfillmentUnfulfilled: 0, entity.FulfillmentPacking: 0, entity.FulfillmentShipped: 0,
			entity.FulfillmentDelivered: 0, entity.FulfillmentCanceled: 0, entity.FulfillmentReturned: 0,
		},
		RiskLevel: map[string]int64{entity.RiskLow: 0, entity.RiskMedium: 0, entity.RiskHigh: 0, entity.RiskUnassessed: 0},
	}
	for _, c := range counts {
		d.ReviewStatus[c.ReviewStatus] += c.Count
		d.FulfillmentStatus[c.FulfillmentStatus] += c.Count
		risk := entity.RiskUnassessed
		if c.RiskLevel != nil {
			risk = *c.RiskLevel
		}
		d.RiskLevel[risk] += c.Count
	}
	return d, nil
}

func (s *sellerServiceImpl) ListListings(ctx context.Context, shopID string, offset, limit int) ([]entity.ListingCard, int64, error) {
	return s.catalogRepo.ListListings(ctx, entity.ListingFilter{ShopID: shopID}, offset, limit)
}

func (s *sellerServiceImpl) getListing(ctx context.Context, shopID, listingID string) (*entity.ListingCard, error) {
	items, _, err := s.catalogRepo.ListListings(ctx, entity.ListingFilter{ShopID: shopID, ListingID: listingID}, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, apperror.ErrNotFound.WithMessage("Không tìm thấy sản phẩm")
	}
	return &items[0], nil
}

// normalizePrice validates a price string and returns it in canonical "123.45" form.
func normalizePrice(price string) (string, error) {
	cents, err := valueobject.ParseCents(price)
	if err != nil {
		return "", apperror.ErrValidation.WithMessage("Giá không hợp lệ (ví dụ: 129.90)")
	}
	return valueobject.FormatCents(cents), nil
}

func (s *sellerServiceImpl) CreateListing(ctx context.Context, cmd service.CreateListingCommand) (*entity.ListingCard, error) {
	name := strings.TrimSpace(cmd.ProductName)
	if name == "" {
		return nil, apperror.ErrValidation.WithMessage("Tên sản phẩm không được để trống")
	}
	price, err := normalizePrice(cmd.CurrentPrice)
	if err != nil {
		return nil, err
	}
	currency := strings.ToUpper(strings.TrimSpace(cmd.Currency))
	if currency == "" {
		currency = "VND"
	}
	if !currencyCode.MatchString(currency) {
		return nil, apperror.ErrValidation.WithMessage("Mã tiền tệ phải gồm 3 ký tự")
	}
	if cmd.StockQuantity != nil && *cmd.StockQuantity < 0 {
		return nil, apperror.ErrValidation.WithMessage("Tồn kho không được âm")
	}

	product := &entity.Product{ProductID: newAppID(), DisplayName: &name, BatchID: entity.AppBatchID}
	product.SourceRecordID = product.ProductID
	if code := strings.TrimSpace(cmd.CategoryCode); code != "" {
		catName, err := s.catalogRepo.FindCategoryName(ctx, code)
		if isNotFound(err) {
			return nil, apperror.ErrValidation.WithMessage("Danh mục không tồn tại")
		}
		if err != nil {
			return nil, err
		}
		product.CategoryCode, product.CategoryNameEn = &code, catName
	}
	listing := &entity.Listing{
		ListingID:     newAppID(),
		ShopID:        cmd.ShopID,
		ProductID:     product.ProductID,
		CurrentPrice:  &price,
		Currency:      currency,
		StockQuantity: cmd.StockQuantity,
		BatchID:       entity.AppBatchID,
	}
	listing.SourceRecordID = listing.ListingID

	// Every listing gets its own product so sellers never edit shared (Olist) products.
	err = s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.catalogRepo.CreateProduct(ctx, product); err != nil {
			return err
		}
		return s.catalogRepo.CreateListing(ctx, listing)
	})
	if err != nil {
		return nil, err
	}

	tlog.Info("Listing created", zap.String("listing_id", listing.ListingID), zap.String("shop_id", cmd.ShopID))
	return s.getListing(ctx, cmd.ShopID, listing.ListingID)
}

func (s *sellerServiceImpl) UpdateListing(ctx context.Context, cmd service.UpdateListingCommand) (*entity.ListingCard, error) {
	var columns []string
	var price string
	if cmd.CurrentPrice != nil {
		var err error
		if price, err = normalizePrice(*cmd.CurrentPrice); err != nil {
			return nil, err
		}
		columns = append(columns, "current_price")
	}
	if cmd.StockQuantity != nil {
		if *cmd.StockQuantity < 0 {
			return nil, apperror.ErrValidation.WithMessage("Tồn kho không được âm")
		}
		columns = append(columns, "stock_quantity")
	}
	if len(columns) == 0 {
		return nil, apperror.ErrValidation.WithMessage("Không có thay đổi")
	}

	// Only the listing changes; order_items keep their price snapshot.
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		listing, err := s.catalogRepo.LockListing(ctx, cmd.ShopID, cmd.ListingID)
		if err != nil {
			return err
		}
		if cmd.CurrentPrice != nil {
			listing.CurrentPrice = &price
		}
		if cmd.StockQuantity != nil {
			listing.StockQuantity = cmd.StockQuantity
		}
		return s.catalogRepo.UpdateListing(ctx, listing, columns...)
	})
	if err != nil {
		return nil, err
	}
	return s.getListing(ctx, cmd.ShopID, cmd.ListingID)
}

func (s *sellerServiceImpl) ListOrders(ctx context.Context, f entity.OrderFilter, offset, limit int) ([]entity.OrderSummary, int64, error) {
	return s.orderRepo.ListOrders(ctx, f, offset, limit)
}

func (s *sellerServiceImpl) GetOrder(ctx context.Context, shopID, orderID string) (*entity.OrderDetail, error) {
	return loadOrderDetail(ctx, s.orderRepo, entity.OrderFilter{ShopID: shopID, OrderID: orderID}, true)
}

func (s *sellerServiceImpl) Decide(ctx context.Context, cmd service.DecisionCommand) (*entity.OrderDetail, error) {
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		// Lock first so a concurrent decision or new assessment cannot slip between check and write.
		order, err := s.orderRepo.LockOrder(ctx, entity.OrderFilter{ShopID: cmd.ShopID, OrderID: cmd.OrderID})
		if err != nil {
			return err
		}
		latest, err := s.orderRepo.FindLatestAssessment(ctx, order.OrderID)
		if err != nil {
			return err
		}
		next, err := checkDecision(order, latest, cmd.AssessmentID, cmd.Action, cmd.Reason)
		if err != nil {
			return err
		}
		sellerID := cmd.SellerID
		if err := s.orderRepo.CreateDecision(ctx, &entity.ReviewDecision{
			BatchID:      order.BatchID,
			DecisionID:   newAppID(),
			OrderID:      order.OrderID,
			AssessmentID: latest.AssessmentID,
			ActorType:    entity.ActorSeller,
			ActorRef:     sellerID,
			SellerID:     &sellerID,
			Action:       cmd.Action,
			Reason:       strings.TrimSpace(cmd.Reason),
			CreatedAt:    time.Now(),
		}); err != nil {
			return err
		}
		order.ReviewStatus = next
		return s.orderRepo.UpdateOrder(ctx, order, "review_status")
	})
	if err != nil {
		return nil, err
	}

	tlog.Info("Review decision recorded", zap.String("order_id", cmd.OrderID), zap.String("action", cmd.Action))
	return s.GetOrder(ctx, cmd.ShopID, cmd.OrderID)
}

func (s *sellerServiceImpl) UpdateFulfillment(ctx context.Context, cmd service.FulfillmentCommand) (*entity.OrderDetail, error) {
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		order, err := s.orderRepo.LockOrder(ctx, entity.OrderFilter{ShopID: cmd.ShopID, OrderID: cmd.OrderID})
		if err != nil {
			return err
		}
		paymentReady := true
		if cmd.Status == entity.FulfillmentPacking {
			if paymentReady, err = s.orderRepo.PaymentReady(ctx, order.CheckoutID); err != nil {
				return err
			}
		}
		if err := checkFulfillment(order, cmd.Status, paymentReady); err != nil {
			return err
		}
		order.FulfillmentStatus = cmd.Status
		if err := s.orderRepo.UpdateOrder(ctx, order, "fulfillment_status"); err != nil {
			return err
		}
		eventType := fulfillmentEvent(cmd.Status)
		if eventType == "" {
			return nil
		}
		eventID := newAppID()
		eventTime := time.Now()
		return s.orderRepo.CreateEvent(ctx, &entity.OrderEvent{
			EventID:        eventID,
			OrderID:        order.OrderID,
			EventType:      eventType,
			ActorType:      entity.ActorSeller,
			OccurredAt:     eventTime,
			BatchID:        order.BatchID,
			AvailableAt:    &eventTime,
			SourceRecordID: eventID,
		})
	})
	if err != nil {
		return nil, err
	}

	tlog.Info("Fulfillment updated", zap.String("order_id", cmd.OrderID), zap.String("status", cmd.Status))
	return s.GetOrder(ctx, cmd.ShopID, cmd.OrderID)
}
