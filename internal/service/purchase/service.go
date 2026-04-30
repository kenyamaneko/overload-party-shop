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

	gamedesign "github.com/kenyamaneko/overload-party-common/packages/game-design-constants"
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/service/subscription"
)

// Service は shop ローカルの購入フローを管理する。shop は `shop` スキーマ
// のみを直接変更し、faction + premium 状態更新は Transactional Outbox 経由で
// account / card / gateway subscriber が処理する。IsOwned チェックは shop ローカル
// の `player_owned_factions` read model で行う。
//
// ビジネス行の INSERT と outbox 行の INSERT は repo 層の単一トランザクションで
// 同時に commit される。publish は別プロセスの worker が outbox を消費して行う。
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
	for _, p := range products {
		owned := false
		switch p.Type {
		case domain.ProductTypeFactionSet:
			var content domain.FactionSetContent
			if err := json.Unmarshal(p.Content, &content); err != nil {
				return nil, fmt.Errorf("parse product content for %s: %w", p.ProductID, err)
			}
			owned = slices.Contains(ownedFactions, content.Faction)
		case domain.ProductTypeSubscription:
			owned = subEntitled
		case domain.ProductTypeCosmetic:
			var content domain.CosmeticContent
			if err := json.Unmarshal(p.Content, &content); err != nil {
				return nil, fmt.Errorf("parse product content for %s: %w", p.ProductID, err)
			}
			owned = slices.ContainsFunc(ownedItems, func(it *domain.PlayerItem) bool {
				return it.ItemType == content.ItemType && it.ItemNo == content.ItemNo
			})
		}
		result = append(result, domain.ProductWithOwnership{Product: *p, IsOwned: owned})
	}
	return result, nil
}

// Purchase は単発購入フローを実行する。レシート検証・べき等チェック・
// 購入記録・faction-purchased イベントの outbox enqueue を行う。
// 購入行 + 所有権行 + outbox 行は repo 層の単一 tx で atomic に commit される。
// publish は worker が outbox を消費して別途実行する。
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

	product, err := s.productRepo.GetProductByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	if !product.IsActive {
		return ErrProductNotActive
	}

	var (
		factionContent  domain.FactionSetContent
		cosmeticContent domain.CosmeticContent
	)

	switch product.Type {
	case domain.ProductTypeFactionSet:
		if err := json.Unmarshal(product.Content, &factionContent); err != nil {
			return fmt.Errorf("parse faction set content: %w", err)
		}
		if !slices.Contains(gamedesign.SelectableFactions, factionContent.Faction) {
			return fmt.Errorf("%w: %s", ErrInvalidFaction, factionContent.Faction)
		}
		ownedFactions, err := s.factionPurchaseRepo.ListOwnedFactions(ctx, playerID)
		if err != nil {
			return fmt.Errorf("check owned factions: %w", err)
		}
		if slices.Contains(ownedFactions, factionContent.Faction) {
			return ErrAlreadyOwned
		}
	case domain.ProductTypeCosmetic:
		if err := json.Unmarshal(product.Content, &cosmeticContent); err != nil {
			return fmt.Errorf("parse cosmetic content: %w", err)
		}
		owned, err := s.itemPurchaseRepo.HasPlayerItem(ctx, playerID, cosmeticContent.ItemType, cosmeticContent.ItemNo)
		if err != nil {
			return fmt.Errorf("check owned item: %w", err)
		}
		if owned {
			return ErrAlreadyOwned
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProductType, product.Type)
	}

	result, err := verifier.VerifyPurchase(ctx, purchaseToken)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerifyReceipt, err)
	}
	if !result.IsValid {
		return ErrReceiptVerificationFailed
	}

	purchase := &domain.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   productID,
		PurchasedAt: time.Now(),
	}

	switch product.Type {
	case domain.ProductTypeFactionSet:
		ev, err := buildFactionPurchasedEvent(playerID, factionContent.Faction)
		if err != nil {
			return fmt.Errorf("build faction-purchased: %w", err)
		}
		if _, err := s.factionPurchaseRepo.CreatePurchase(ctx, purchase, factionContent.Faction, pf, purchaseToken, ev); err != nil {
			return fmt.Errorf("create faction purchase: %w", err)
		}
	case domain.ProductTypeCosmetic:
		item := &domain.PlayerItem{
			PlayerID:   playerID,
			ItemType:   cosmeticContent.ItemType,
			ItemNo:     cosmeticContent.ItemNo,
			AcquiredAt: time.Now(),
		}
		if _, err := s.itemPurchaseRepo.CreatePurchase(ctx, purchase, item, pf, purchaseToken); err != nil {
			return fmt.Errorf("create item purchase: %w", err)
		}
	}

	return nil
}

// Subscribe はサブスクリプション購入フローを実行する。レシート検証・べき等チェック・
// サブスクリプション記録・premium-updated イベントの outbox enqueue を行う。
// subscription 行 + token 行 + outbox 行は repo 層の単一 tx で atomic に commit される。
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

	product, err := s.productRepo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if product.Type != domain.ProductTypeSubscription {
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
	ev := domain.FactionPurchasedEvent{
		EventType: domain.EventTypeFactionPurchased,
		EventID:   eventID.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
	}
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
	ev := domain.PremiumUpdatedEvent{
		EventType:        domain.EventTypePremiumUpdated,
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           domain.PremiumUpdatedSourceShop,
	}
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
