package purchase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// Service は shop ローカルの購入フロー (商品一覧・単発購入・サブスクリプション) を管理する。
type Service struct {
	productRepo         port.ProductRepo
	factionPurchaseRepo port.FactionPurchaseRepo
	itemPurchaseRepo    port.ItemPurchaseRepo
	purchaseLookup      port.PurchaseLookupRepo
	subRepo             port.SubscriptionRepo
	appleVerifier       port.ReceiptVerifier
	googleVerifier      port.ReceiptVerifier
}

func New(
	productRepo port.ProductRepo,
	factionPurchaseRepo port.FactionPurchaseRepo,
	itemPurchaseRepo port.ItemPurchaseRepo,
	purchaseLookup port.PurchaseLookupRepo,
	subRepo port.SubscriptionRepo,
	appleVerifier port.ReceiptVerifier,
	googleVerifier port.ReceiptVerifier,
) *Service {
	return &Service{
		productRepo:         productRepo,
		factionPurchaseRepo: factionPurchaseRepo,
		itemPurchaseRepo:    itemPurchaseRepo,
		purchaseLookup:      purchaseLookup,
		subRepo:             subRepo,
		appleVerifier:       appleVerifier,
		googleVerifier:      googleVerifier,
	}
}

// GetProducts はプレイヤー向けの商品一覧を所有状態付きで返す。
func (s *Service) GetProducts(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error) {
	products, err := s.productRepo.GetActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get products: %w", err)
	}

	ownedFactions, err := s.factionPurchaseRepo.ListOwnedFactions(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list owned factions: %w", err)
	}

	latestSub, err := s.subRepo.GetLatestSubscription(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	subEntitled, err := subscription.IsEntitled(latestSub, time.Now())
	if err != nil {
		return nil, fmt.Errorf("check subscription entitlement: %w", err)
	}

	ownedItems, err := s.itemPurchaseRepo.ListPlayerItems(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list owned items: %w", err)
	}

	result := make([]domain.ProductWithOwnership, 0, len(products))
	for _, pv := range products {
		owned := false
		switch p := pv.(type) {
		case domain.FactionSetProduct:
			owned = slices.Contains(ownedFactions, p.Faction)
		case domain.SubscriptionProduct:
			owned = subEntitled
		case domain.CosmeticProduct:
			owned = slices.ContainsFunc(ownedItems, func(it *domain.PlayerItem) bool {
				return it.ItemType == p.ItemType && it.ItemNo == p.ItemNo
			})
		}
		result = append(result, domain.ProductWithOwnership{ProductView: pv, IsOwned: owned})
	}
	return result, nil
}

// Purchase は単発購入フローを実行する (べき等チェック・レシート検証・購入記録・outbox enqueue)。
func (s *Service) Purchase(ctx context.Context, playerID, productID, pf, purchaseToken string) error {
	verifier, err := s.getVerifier(pf)
	if err != nil {
		return err
	}

	existing, err := s.purchaseLookup.FindPurchaseByToken(ctx, pf, purchaseToken)
	if err != nil {
		return fmt.Errorf("check existing purchase: %w", err)
	}
	if existing != nil {
		slog.Info("purchase idempotent skip", "player_id", playerID, "product_id", productID, "platform", pf)
		return nil
	}

	pv, err := s.productRepo.GetProductByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	if !pv.Common().IsActive {
		return ErrProductNotActive
	}

	switch p := pv.(type) {
	case domain.FactionSetProduct:
		return s.purchaseFactionSet(ctx, playerID, p, pf, purchaseToken, verifier)
	case domain.CosmeticProduct:
		return s.purchaseCosmetic(ctx, playerID, p, pf, purchaseToken, verifier)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProductType, pv.Common().Type)
	}
}

func (s *Service) purchaseFactionSet(ctx context.Context, playerID string, product domain.FactionSetProduct, pf, purchaseToken string, verifier port.ReceiptVerifier) error {
	// product_faction_grants の DB CHECK が selectable faction のみを許容するため、ここでの値域検査は不要。
	ownedFactions, err := s.factionPurchaseRepo.ListOwnedFactions(ctx, playerID)
	if err != nil {
		return fmt.Errorf("check owned factions: %w", err)
	}
	if slices.Contains(ownedFactions, product.Faction) {
		return ErrAlreadyOwned
	}

	if err := verifyPurchase(ctx, verifier, purchaseToken); err != nil {
		return err
	}

	purchase := &domain.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   product.Product.ProductID,
		PurchasedAt: time.Now(),
	}
	ev, err := buildFactionPurchasedEvent(playerID, product.Faction)
	if err != nil {
		return fmt.Errorf("build faction-purchased: %w", err)
	}
	if _, err := s.factionPurchaseRepo.CreatePurchase(ctx, purchase, product.Faction, pf, purchaseToken, ev); err != nil {
		return fmt.Errorf("create faction purchase: %w", err)
	}
	return nil
}

func (s *Service) purchaseCosmetic(ctx context.Context, playerID string, product domain.CosmeticProduct, pf, purchaseToken string, verifier port.ReceiptVerifier) error {
	owned, err := s.itemPurchaseRepo.HasPlayerItem(ctx, playerID, product.ItemType, product.ItemNo)
	if err != nil {
		return fmt.Errorf("check owned item: %w", err)
	}
	if owned {
		return ErrAlreadyOwned
	}

	if err := verifyPurchase(ctx, verifier, purchaseToken); err != nil {
		return err
	}

	purchase := &domain.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   product.Product.ProductID,
		PurchasedAt: time.Now(),
	}
	item := &domain.PlayerItem{
		PlayerID:   playerID,
		ItemType:   product.ItemType,
		ItemNo:     product.ItemNo,
		AcquiredAt: time.Now(),
	}
	if _, err := s.itemPurchaseRepo.CreatePurchase(ctx, purchase, item, pf, purchaseToken); err != nil {
		return fmt.Errorf("create item purchase: %w", err)
	}
	return nil
}

func verifyPurchase(ctx context.Context, verifier port.ReceiptVerifier, purchaseToken string) error {
	result, err := verifier.VerifyPurchase(ctx, purchaseToken)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerifyReceipt, err)
	}
	if !result.IsValid {
		return ErrReceiptVerificationFailed
	}
	return nil
}

// Subscribe はサブスクリプション購入フローを実行する (べき等チェック・レシート検証・記録・outbox enqueue)。
func (s *Service) Subscribe(ctx context.Context, playerID, productID, pf, purchaseToken string) (*time.Time, error) {
	verifier, err := s.getVerifier(pf)
	if err != nil {
		return nil, err
	}

	existing, err := s.subRepo.FindSubscriptionByToken(ctx, pf, purchaseToken)
	if err != nil {
		return nil, fmt.Errorf("check existing subscription: %w", err)
	}
	if existing != nil {
		slog.Info("subscribe idempotent skip", "player_id", playerID, "product_id", productID, "platform", pf)
		return &existing.CurrentPeriodEnd, nil
	}

	pv, err := s.productRepo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if _, ok := pv.(domain.SubscriptionProduct); !ok {
		return nil, ErrProductNotSubscription
	}
	info, err := verifier.VerifySubscription(ctx, purchaseToken)
	if err != nil {
		return nil, fmt.Errorf("verify subscription: %w", err)
	}
	if !info.IsValid {
		return nil, ErrSubVerificationFailed
	}

	now := time.Now()
	sub := &domain.Subscription{
		PlayerID:           playerID,
		ProductID:          productID,
		Status:             domain.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   info.ExpiresAt,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	ev, err := buildPremiumUpdatedEvent(playerID, true, &info.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("build premium-updated: %w", err)
	}
	if err := s.subRepo.CreateSubscription(ctx, sub, pf, purchaseToken, ev); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	return &info.ExpiresAt, nil
}

func (s *Service) getVerifier(pf string) (port.ReceiptVerifier, error) {
	switch pf {
	case domain.PlatformIOS:
		return s.appleVerifier, nil
	case domain.PlatformAndroid:
		return s.googleVerifier, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, pf)
	}
}

func buildFactionPurchasedEvent(playerID, faction string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("purchase: playerID is empty")
	}
	if faction == "" {
		return port.OutboxEvent{}, errors.New("purchase: faction is empty")
	}
	eventID := uuid.New()
	ev := presenter.ToFactionPurchasedEvent(eventID.String(), playerID, faction, time.Now().UTC())
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal faction-purchased: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: domain.EventTypeFactionPurchased,
		Payload:   payload,
	}, nil
}

func buildPremiumUpdatedEvent(playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("purchase: playerID is empty")
	}
	eventID := uuid.New()
	ev := presenter.ToPremiumUpdatedEvent(eventID.String(), playerID, isPremium, expiresAt, time.Now().UTC())
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal premium-updated: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: domain.EventTypePremiumUpdated,
		Payload:   payload,
	}, nil
}
