package purchase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// Service は shop ローカルの購入フロー (商品一覧・単発購入・サブスクリプション) を管理する。
type Service struct {
	productRepo          port.ProductRepo
	factionPurchaseRepo  port.FactionPurchaseRepo
	cardPackPurchaseRepo port.CardPackPurchaseRepo
	itemPurchaseRepo     port.ItemPurchaseRepo
	purchaseLookup       port.PurchaseLookupRepo
	subscriptionRepo     port.SubscriptionRepo
	appleVerifier        port.ReceiptVerifier
	googleVerifier       port.ReceiptVerifier
}

func New(
	productRepo port.ProductRepo,
	factionPurchaseRepo port.FactionPurchaseRepo,
	cardPackPurchaseRepo port.CardPackPurchaseRepo,
	itemPurchaseRepo port.ItemPurchaseRepo,
	purchaseLookup port.PurchaseLookupRepo,
	subscriptionRepo port.SubscriptionRepo,
	appleVerifier port.ReceiptVerifier,
	googleVerifier port.ReceiptVerifier,
) *Service {
	return &Service{
		productRepo:          productRepo,
		factionPurchaseRepo:  factionPurchaseRepo,
		cardPackPurchaseRepo: cardPackPurchaseRepo,
		itemPurchaseRepo:     itemPurchaseRepo,
		purchaseLookup:       purchaseLookup,
		subscriptionRepo:     subscriptionRepo,
		appleVerifier:        appleVerifier,
		googleVerifier:       googleVerifier,
	}
}

// GetProducts はプレイヤー向けの商品一覧を所有状態付きで返す。
func (s *Service) GetProducts(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error) {
	products, err := s.productRepo.GetActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get products: %w", err)
	}

	latestSubscription, err := s.subscriptionRepo.GetLatestSubscription(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	isSubscriptionEntitled, err := subscription.IsEntitled(latestSubscription, time.Now())
	if err != nil {
		return nil, fmt.Errorf("check subscription entitlement: %w", err)
	}

	ownedItems, err := s.itemPurchaseRepo.ListPlayerItems(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list owned items: %w", err)
	}

	result := make([]domain.ProductWithOwnership, 0, len(products))
	for _, productView := range products {
		owned, err := s.isProductOwned(ctx, playerID, productView, ownedItems, isSubscriptionEntitled)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.ProductWithOwnership{ProductView: productView, IsOwned: owned})
	}
	return result, nil
}

// isProductOwned は per-type ProductView ごとに所有状態を判定する。
// faction_set / card_pack は再購入禁止契約が card_pack_id 単位なので
// player_owned_card_packs を引く (faction 所有とは別軸)。
func (s *Service) isProductOwned(ctx context.Context, playerID string, productView domain.ProductView, ownedItems []*domain.PlayerItem, isSubscriptionEntitled bool) (bool, error) {
	switch p := productView.(type) {
	case domain.FactionSetProduct:
		return s.cardPackPurchaseRepo.HasPlayerCardPack(ctx, playerID, p.CardPackID)
	case domain.CardPackProduct:
		return s.cardPackPurchaseRepo.HasPlayerCardPack(ctx, playerID, p.CardPackID)
	case domain.SubscriptionProduct:
		return isSubscriptionEntitled, nil
	case domain.CosmeticProduct:
		for _, it := range ownedItems {
			if it.ItemType == p.ItemType && it.ItemNo == p.ItemNo {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, fmt.Errorf("isProductOwned: unknown product view type %T", productView)
	}
}

// Purchase は単発購入フローを実行する (べき等チェック・レシート検証・購入記録・outbox enqueue)。
func (s *Service) Purchase(ctx context.Context, playerID, productID, platform, purchaseToken string) error {
	verifier, err := s.getVerifier(platform)
	if err != nil {
		return err
	}

	existing, err := s.purchaseLookup.FindPurchaseByToken(ctx, platform, purchaseToken)
	if err != nil {
		return fmt.Errorf("check existing purchase: %w", err)
	}
	if existing != nil {
		slog.Info("purchase idempotent skip", "player_id", playerID, "product_id", productID, "platform", platform)
		return nil
	}

	productView, err := s.productRepo.GetProductByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	if !productView.Common().IsActive {
		return ErrProductNotActive
	}

	switch p := productView.(type) {
	case domain.FactionSetProduct:
		return s.purchaseFactionSet(ctx, playerID, p, platform, purchaseToken, verifier)
	case domain.CardPackProduct:
		return s.purchaseCardPack(ctx, playerID, p, platform, purchaseToken, verifier)
	case domain.CosmeticProduct:
		return s.purchaseCosmetic(ctx, playerID, p, platform, purchaseToken, verifier)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProductType, productView.Common().Type)
	}
}

func (s *Service) purchaseFactionSet(ctx context.Context, playerID string, product domain.FactionSetProduct, platform, purchaseToken string, verifier port.ReceiptVerifier) error {
	owned, err := s.cardPackPurchaseRepo.HasPlayerCardPack(ctx, playerID, product.CardPackID)
	if err != nil {
		return fmt.Errorf("check owned card pack: %w", err)
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
	cardPackEvent, err := buildCardPackPurchasedEvent(playerID, product.CardPackID)
	if err != nil {
		return fmt.Errorf("build card-pack-purchased: %w", err)
	}
	factionEvent, err := buildFactionAcquiredEvent(playerID, product.Faction)
	if err != nil {
		return fmt.Errorf("build faction-acquired: %w", err)
	}
	events := []port.OutboxEvent{cardPackEvent, factionEvent}
	if _, err := s.factionPurchaseRepo.CreatePurchase(ctx, purchase, product.Faction, product.CardPackID, platform, purchaseToken, events); err != nil {
		return fmt.Errorf("create faction purchase: %w", err)
	}
	return nil
}

func (s *Service) purchaseCardPack(ctx context.Context, playerID string, product domain.CardPackProduct, platform, purchaseToken string, verifier port.ReceiptVerifier) error {
	owned, err := s.cardPackPurchaseRepo.HasPlayerCardPack(ctx, playerID, product.CardPackID)
	if err != nil {
		return fmt.Errorf("check owned card pack: %w", err)
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
	event, err := buildCardPackPurchasedEvent(playerID, product.CardPackID)
	if err != nil {
		return fmt.Errorf("build card-pack-purchased: %w", err)
	}
	if _, err := s.cardPackPurchaseRepo.CreatePurchase(ctx, purchase, product.CardPackID, platform, purchaseToken, event); err != nil {
		return fmt.Errorf("create card pack purchase: %w", err)
	}
	return nil
}

func (s *Service) purchaseCosmetic(ctx context.Context, playerID string, product domain.CosmeticProduct, platform, purchaseToken string, verifier port.ReceiptVerifier) error {
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
	if _, err := s.itemPurchaseRepo.CreatePurchase(ctx, purchase, item, platform, purchaseToken); err != nil {
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
func (s *Service) Subscribe(ctx context.Context, playerID, productID, platform, purchaseToken string) (*time.Time, error) {
	verifier, err := s.getVerifier(platform)
	if err != nil {
		return nil, err
	}

	existing, err := s.subscriptionRepo.FindSubscriptionByToken(ctx, platform, purchaseToken)
	if err != nil {
		return nil, fmt.Errorf("check existing subscription: %w", err)
	}
	if existing != nil {
		slog.Info("subscribe idempotent skip", "player_id", playerID, "product_id", productID, "platform", platform)
		return &existing.CurrentPeriodEnd, nil
	}

	productView, err := s.productRepo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if _, ok := productView.(domain.SubscriptionProduct); !ok {
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
	subscription := &domain.Subscription{
		PlayerID:           playerID,
		ProductID:          productID,
		Status:             domain.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   info.ExpiresAt,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	event, err := buildPremiumUpdatedEvent(playerID, true, &info.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("build premium-updated: %w", err)
	}
	if err := s.subscriptionRepo.CreateSubscription(ctx, subscription, platform, purchaseToken, event); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	return &info.ExpiresAt, nil
}

func (s *Service) getVerifier(platform string) (port.ReceiptVerifier, error) {
	switch platform {
	case domain.PlatformIOS:
		return s.appleVerifier, nil
	case domain.PlatformAndroid:
		return s.googleVerifier, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, platform)
	}
}

func buildCardPackPurchasedEvent(playerID, cardPackID string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("purchase: playerID is empty")
	}
	if cardPackID == "" {
		return port.OutboxEvent{}, errors.New("purchase: cardPackID is empty")
	}
	eventID := uuid.New()
	cardPackPurchased := domain.CardPackPurchasedEvent{
		EventID:    eventID.String(),
		Timestamp:  time.Now().UTC(),
		PlayerID:   playerID,
		CardPackID: cardPackID,
	}
	eventType, payload, err := presenter.ToCardPackPurchasedWire(cardPackPurchased)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("present card-pack-purchased: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: eventType,
		Payload:   payload,
	}, nil
}

func buildFactionAcquiredEvent(playerID, faction string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("purchase: playerID is empty")
	}
	if faction == "" {
		return port.OutboxEvent{}, errors.New("purchase: faction is empty")
	}
	eventID := uuid.New()
	factionAcquired := domain.FactionAcquiredEvent{
		EventID:   eventID.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
	}
	eventType, payload, err := presenter.ToFactionAcquiredWire(factionAcquired)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("present faction-acquired: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: eventType,
		Payload:   payload,
	}, nil
}

func buildPremiumUpdatedEvent(playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("purchase: playerID is empty")
	}
	eventID := uuid.New()
	premiumUpdated := domain.PremiumUpdatedEvent{
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
	}
	eventType, payload, err := presenter.ToPremiumUpdatedWire(premiumUpdated)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("present premium-updated: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: eventType,
		Payload:   payload,
	}, nil
}
