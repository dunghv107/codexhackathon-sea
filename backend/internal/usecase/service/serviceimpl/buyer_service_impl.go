package serviceimpl

import (
	"context"
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

var buyerPaymentMethods = map[string]bool{
	"cod": true, "credit_card": true, "debit_card": true, "boleto": true, "voucher": true,
}

type buyerServiceImpl struct {
	addressRepo  repository.AddressRepository
	catalogRepo  repository.CatalogRepository
	orderRepo    repository.OrderRepository
	tx           repository.Transactor
	risk         service.RiskClient
	phone        *PhoneIdentityService
	verification *VerificationService
}

// NewBuyerService creates a new buyer service
func NewBuyerService(
	addressRepo repository.AddressRepository,
	catalogRepo repository.CatalogRepository,
	orderRepo repository.OrderRepository,
	tx repository.Transactor,
	risk service.RiskClient,
	phone *PhoneIdentityService,
	verification *VerificationService,
) service.BuyerService {
	return &buyerServiceImpl{addressRepo: addressRepo, catalogRepo: catalogRepo, orderRepo: orderRepo, tx: tx, risk: risk, phone: phone, verification: verification}
}

// addressKey normalizes an address for matching: lowercase, trimmed, single spaces.
func addressKey(raw string) string {
	return strings.Join(strings.Fields(strings.ToLower(raw)), " ")
}

func (s *buyerServiceImpl) ListAddresses(ctx context.Context, buyerID string) ([]entity.SavedAddress, error) {
	return s.addressRepo.ListByBuyer(ctx, buyerID)
}

func (s *buyerServiceImpl) SaveAddress(ctx context.Context, cmd service.SaveAddressCommand) (*entity.SavedAddress, error) {
	label, raw, region := strings.TrimSpace(cmd.Label), strings.TrimSpace(cmd.AddressRaw), strings.TrimSpace(cmd.Region)
	country := strings.ToUpper(strings.TrimSpace(cmd.CountryCode))
	if label == "" || raw == "" || region == "" {
		return nil, apperror.ErrValidation.WithMessage("Nhãn, địa chỉ và tỉnh/thành không được để trống")
	}
	if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
		return nil, apperror.ErrValidation.WithMessage("Mã quốc gia phải gồm 2 chữ cái")
	}

	var addr *entity.SavedAddress
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if cmd.ID == "" {
			addr = &entity.SavedAddress{
				SavedAddressID: newAppID(),
				BuyerID:        cmd.BuyerID,
				CreatedAt:      time.Now(),
				BatchID:        entity.DemoBatchID,
			}
		} else {
			var err error
			if addr, err = s.addressRepo.FindOwned(ctx, cmd.BuyerID, cmd.ID); err != nil {
				return err
			}
		}
		addr.Label, addr.AddressRaw, addr.AddressKey = label, raw, addressKey(raw)
		addr.Region, addr.CountryCode, addr.IsDefault = region, country, cmd.IsDefault

		// At most one default per buyer: unset the old one first (partial unique index).
		if addr.IsDefault {
			if err := s.addressRepo.ClearDefault(ctx, cmd.BuyerID, addr.SavedAddressID); err != nil {
				return err
			}
		}
		if cmd.ID == "" {
			return s.addressRepo.Create(ctx, addr)
		}
		return s.addressRepo.Update(ctx, addr)
	})
	if err != nil {
		return nil, err
	}
	return addr, nil
}

func (s *buyerServiceImpl) DeleteAddress(ctx context.Context, buyerID, id string) error {
	return s.addressRepo.Delete(ctx, buyerID, id)
}

func (s *buyerServiceImpl) Checkout(ctx context.Context, cmd service.CheckoutCommand) (*service.CheckoutResult, error) {
	if !buyerPaymentMethods[cmd.PaymentMethod] {
		return nil, apperror.ErrValidation.WithMessage("Phương thức thanh toán không hợp lệ")
	}
	if len(cmd.Items) == 0 {
		return nil, apperror.ErrValidation.WithMessage("Giỏ hàng trống")
	}
	ids := make([]string, 0, len(cmd.Items))
	seen := make(map[string]bool, len(cmd.Items))
	for _, it := range cmd.Items {
		if it.Quantity < 1 || it.Quantity > 99 {
			return nil, apperror.ErrValidation.WithMessage("Số lượng phải từ 1 đến 99")
		}
		if seen[it.ListingID] {
			return nil, apperror.ErrValidation.WithMessage("Sản phẩm bị lặp trong đơn")
		}
		seen[it.ListingID] = true
		ids = append(ids, it.ListingID)
	}

	var result *service.CheckoutResult
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		contact, err := s.phone.Resolve(ctx, cmd.Phone)
		if err != nil {
			return err
		}
		savedAddr, err := s.addressRepo.FindOwned(ctx, cmd.BuyerID, cmd.SavedAddressID)
		if err != nil {
			return err
		}
		locked, err := s.catalogRepo.LockListings(ctx, ids)
		if err != nil {
			return err
		}
		listings := make(map[string]entity.ListingCard, len(locked))
		for _, l := range locked {
			listings[l.ListingID] = l
		}

		// Validate every line and price it in cents before writing anything.
		now := time.Now()
		checkoutID := newAppID()
		var currency string
		var checkoutTotal int64
		var shopOrder []string
		orders := map[string]*entity.Order{}
		orderItems := map[string][]entity.OrderItem{}
		orderTotals := map[string]int64{}
		shopNames := map[string]string{}
		for _, it := range cmd.Items {
			l, ok := listings[it.ListingID]
			if !ok {
				return apperror.ErrNotFound.WithMessage("Không tìm thấy sản phẩm")
			}
			if l.BatchID != entity.DemoBatchID || l.ShopBatchID != entity.DemoBatchID || l.ProductBatchID != entity.DemoBatchID {
				return service.ErrRiskBatch
			}
			if l.CurrentPrice == nil {
				return apperror.ErrValidation.WithMessage("Sản phẩm chưa có giá bán")
			}
			if currency == "" {
				currency = l.Currency
			} else if l.Currency != currency {
				return service.ErrMixedCurrency
			}
			if l.StockQuantity != nil {
				if *l.StockQuantity < it.Quantity {
					return service.ErrOutOfStock
				}
				left := *l.StockQuantity - it.Quantity
				if err := s.catalogRepo.UpdateListing(ctx, &entity.Listing{ListingID: l.ListingID, StockQuantity: &left}, "stock_quantity"); err != nil {
					return err
				}
			}
			unit, err := valueobject.ParseCents(*l.CurrentPrice)
			if err != nil {
				return apperror.ErrInternalServerError.WithMessage("Giá sản phẩm không hợp lệ").WithError(err)
			}

			order, ok := orders[l.ShopID]
			if !ok {
				order = &entity.Order{
					OrderID:           newAppID(),
					CheckoutID:        checkoutID,
					ShopID:            l.ShopID,
					FulfillmentStatus: entity.FulfillmentUnfulfilled,
					ReviewStatus:      entity.ReviewPending,
					BatchID:           entity.DemoBatchID,
				}
				order.SourceRecordID = order.OrderID
				orders[l.ShopID], shopNames[l.ShopID] = order, l.ShopName
				shopOrder = append(shopOrder, l.ShopID)
			}
			itemID := newAppID()
			orderItems[l.ShopID] = append(orderItems[l.ShopID], entity.OrderItem{
				ProductName:    l.ProductName,
				ItemID:         itemID,
				OrderID:        order.OrderID,
				ShopID:         l.ShopID,
				ListingID:      l.ListingID,
				Quantity:       it.Quantity,
				UnitPrice:      valueobject.FormatCents(unit),
				FreightAmount:  valueobject.FormatCents(0), // no freight source yet
				BatchID:        entity.DemoBatchID,
				SourceRecordID: itemID,
			})
			if checkoutTotal, err = valueobject.AddLineCents(checkoutTotal, unit, it.Quantity); err != nil {
				return apperror.ErrValidation.WithMessage("Tổng tiền vượt giới hạn")
			}
			orderTotals[l.ShopID] += int64(it.Quantity) * unit // bounded by checkoutTotal
		}

		checkout := entity.Checkout{
			CheckoutID:     checkoutID,
			BuyerID:        cmd.BuyerID,
			PlacedAt:       now,
			PaymentMethod:  cmd.PaymentMethod,
			Currency:       currency,
			TotalAmount:    valueobject.FormatCents(checkoutTotal),
			BatchID:        entity.DemoBatchID,
			SourceRecordID: checkoutID,
		}
		region, country := savedAddr.Region, savedAddr.CountryCode
		snapshot := &entity.CheckoutAddress{
			CheckoutID:     checkoutID,
			AddressRaw:     &savedAddr.AddressRaw,
			AddressKey:     &savedAddr.AddressKey,
			Region:         &region,
			CountryCode:    &country,
			BatchID:        entity.DemoBatchID,
			SourceRecordID: checkoutID,
		}
		paymentID := newAppID()
		payment := &entity.Payment{
			PaymentID:      paymentID,
			CheckoutID:     checkoutID,
			Method:         cmd.PaymentMethod,
			Amount:         checkout.TotalAmount,
			Status:         entity.PaymentPending,
			BatchID:        entity.DemoBatchID,
			SourceRecordID: paymentID,
		}
		contact.CheckoutID, contact.CreatedAt = checkoutID, now
		if err := s.orderRepo.CreateCheckout(ctx, &checkout, snapshot, payment, contact); err != nil {
			return err
		}

		result = &service.CheckoutResult{Checkout: checkout}
		for _, shopID := range shopOrder {
			order := orders[shopID]
			if err := s.orderRepo.CreateOrder(ctx, order, orderItems[shopID]); err != nil {
				return err
			}
			eventID := newAppID()
			if err := s.orderRepo.CreateEvent(ctx, &entity.OrderEvent{
				EventID:        eventID,
				OrderID:        order.OrderID,
				EventType:      entity.EventCreated,
				ActorType:      entity.ActorBuyer,
				OccurredAt:     now,
				AvailableAt:    &now,
				BatchID:        entity.DemoBatchID,
				SourceRecordID: eventID,
			}); err != nil {
				return err
			}
			result.Orders = append(result.Orders, service.CheckoutOrder{
				OrderID:    order.OrderID,
				ShopID:     shopID,
				ShopName:   shopNames[shopID],
				OrderTotal: valueobject.FormatCents(orderTotals[shopID]),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The checkout is committed before KBS reads it; failure must not cause a duplicate purchase retry.
	// Bound the whole multi-shop scoring attempt below the HTTP server's write timeout.
	scoreCtx, cancelScoring := context.WithTimeout(ctx, 5*time.Second)
	defer cancelScoring()
	for _, order := range result.Orders {
		if _, err := s.risk.Score(scoreCtx, order.ShopID, order.OrderID, false); err != nil {
			tlog.Warn("Scoring unavailable; seller can retry", zap.String("order_id", order.OrderID))
		} else if err := s.verification.EnsureForOrder(scoreCtx, order.ShopID, order.OrderID); err != nil {
			tlog.Warn("Verification unavailable; seller can retry", zap.String("order_id", order.OrderID))
		}
		if scoreCtx.Err() != nil {
			break
		}
	}

	tlog.Info("Checkout placed", zap.String("checkout_id", result.Checkout.CheckoutID), zap.Int("orders", len(result.Orders)))
	return result, nil
}

func (s *buyerServiceImpl) ListOrders(ctx context.Context, buyerID, status string, offset, limit int) ([]entity.OrderSummary, int64, error) {
	switch status {
	case "", "confirming", "confirmed", "packing", "shipped", "delivered", "canceled", "returned":
	default:
		return nil, 0, apperror.ErrValidation.WithMessage("Trạng thái đơn không hợp lệ")
	}
	return s.orderRepo.ListOrders(ctx, entity.OrderFilter{BuyerID: buyerID, DisplayStatus: status}, offset, limit)
}

func (s *buyerServiceImpl) GetOrder(ctx context.Context, buyerID, orderID string) (*entity.OrderDetail, error) {
	return loadOrderDetail(ctx, s.orderRepo, entity.OrderFilter{BuyerID: buyerID, OrderID: orderID}, false)
}

func (s *buyerServiceImpl) CancelOrder(ctx context.Context, buyerID, orderID, reason string) (*entity.OrderDetail, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, apperror.ErrValidation.WithMessage("Lý do không được để trống")
	}
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		order, err := s.orderRepo.LockOrder(ctx, entity.OrderFilter{BuyerID: buyerID, OrderID: orderID})
		if err != nil {
			return err
		}
		if order.FulfillmentStatus != entity.FulfillmentUnfulfilled {
			return service.ErrInvalidTransition
		}
		// shortcut: stock is not returned to listings on cancel, add a restock step when stock matters beyond the demo.
		order.FulfillmentStatus = entity.FulfillmentCanceled
		if err := s.orderRepo.UpdateOrder(ctx, order, "fulfillment_status"); err != nil {
			return err
		}
		eventID := newAppID()
		eventTime := time.Now()
		return s.orderRepo.CreateEvent(ctx, &entity.OrderEvent{
			EventID:        eventID,
			OrderID:        order.OrderID,
			EventType:      entity.EventCanceled,
			ActorType:      entity.ActorBuyer,
			Reason:         &reason,
			OccurredAt:     eventTime,
			AvailableAt:    &eventTime,
			BatchID:        order.BatchID,
			SourceRecordID: eventID,
		})
	})
	if err != nil {
		return nil, err
	}

	tlog.Info("Order canceled by buyer", zap.String("order_id", orderID))
	return s.GetOrder(ctx, buyerID, orderID)
}
