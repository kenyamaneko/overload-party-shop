package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"time"

	gamedesign "github.com/kenyamaneko/overload-party-common/packages/game-design-constants"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/platform"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// CardLister は shop が faction-set カード列挙に必要とする card サービスの狭い
// スライス。商品カタログメタデータ層でのみ使用。カード付与には使用しない。
type CardLister interface {
	ListAllCards(ctx context.Context) ([]*apishop.CardView, error)
}

// ShopService は shop ローカルの購入フローを管理する。Pub/Sub fan-out リファクタ以降、
// shop は `shop` スキーマのみを直接変更する。faction + premium 状態更新は Pub/Sub
// 経由で account / card / gateway subscriber が処理する。IsOwned チェックは
// shop ローカルの `player_owned_factions` read model で行う。
type ShopService struct {
	shopRepo         port.ShopRepository
	subRepo          port.SubscriptionRepo
	ownedFactionRepo port.OwnedFactionRepo
	txRunner         port.TxRunner
	cardLister       CardLister
	appleVerifier    platform.ReceiptVerifier
	googleVerifier   platform.ReceiptVerifier
	factionPublisher port.FactionEventPublisher
	premiumPublisher port.PremiumEventPublisher
}

// NewShopService は依存を受け取り ShopService を構築する。
func NewShopService(
	shopRepo port.ShopRepository,
	subRepo port.SubscriptionRepo,
	ownedFactionRepo port.OwnedFactionRepo,
	txRunner port.TxRunner,
	cardLister CardLister,
	appleVerifier platform.ReceiptVerifier,
	googleVerifier platform.ReceiptVerifier,
	factionPublisher port.FactionEventPublisher,
	premiumPublisher port.PremiumEventPublisher,
) *ShopService {
	return &ShopService{
		shopRepo:         shopRepo,
		subRepo:          subRepo,
		ownedFactionRepo: ownedFactionRepo,
		txRunner:         txRunner,
		cardLister:       cardLister,
		appleVerifier:    appleVerifier,
		googleVerifier:   googleVerifier,
		factionPublisher: factionPublisher,
		premiumPublisher: premiumPublisher,
	}
}

// GetProducts はプレイヤー向けの商品一覧を所有状態付きで返す。
func (s *ShopService) GetProducts(ctx context.Context, playerID string) ([]apishop.ProductResponse, error) {
	products, err := s.shopRepo.GetActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get products: %w", err)
	}

	ownedFactions, err := s.ownedFactionRepo.List(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list owned factions: %w", err)
	}

	latestSub, err := s.subRepo.GetLatestSubscription(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	subEntitled := isEntitled(latestSub, time.Now())

	ownedItems, err := s.shopRepo.ListPlayerItems(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list owned items: %w", err)
	}
	type itemKey struct {
		Type string
		No   int64
	}
	ownedItemSet := make(map[itemKey]bool, len(ownedItems))
	for _, it := range ownedItems {
		ownedItemSet[itemKey{it.ItemType, it.ItemNo}] = true
	}

	result := make([]apishop.ProductResponse, 0, len(products))
	for _, p := range products {
		owned := false
		switch p.Type {
		case apishop.ProductTypeFactionSet:
			var content apishop.FactionSetContent
			if err := json.Unmarshal(p.Content, &content); err != nil {
				log.Printf("failed to parse product content for %s: %v", p.ProductID, err)
				return nil, fmt.Errorf("parse product content for %s: %w", p.ProductID, err)
			}
			owned = slices.Contains(ownedFactions, content.Faction)
		case apishop.ProductTypeSubscription:
			owned = subEntitled
		case apishop.ProductTypeCosmetic:
			var content apishop.CosmeticContent
			if err := json.Unmarshal(p.Content, &content); err != nil {
				log.Printf("failed to parse product content for %s: %v", p.ProductID, err)
				return nil, fmt.Errorf("parse product content for %s: %w", p.ProductID, err)
			}
			owned = ownedItemSet[itemKey{content.ItemType, content.ItemNo}]
		}
		result = append(result, apishop.ProductResponse{
			ProductID:   p.ProductID,
			Name:        p.Name,
			Type:        p.Type,
			Price:       p.Price,
			Content:     p.Content,
			Description: p.Description,
			ImageURL:    p.ImageURL,
			IsActive:    p.IsActive,
			IsOwned:     owned,
		})
	}
	return result, nil
}

// Purchase は単発購入フローを実行する。レシート検証・べき等チェック・購入記録・
// faction-selected イベント発行を行う。
func (s *ShopService) Purchase(ctx context.Context, playerID, productID, pf, purchaseToken string) error {
	verifier := s.getVerifier(pf)
	if verifier == nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedPlatform, pf)
	}

	existing, err := s.shopRepo.FindPurchaseByToken(ctx, pf, purchaseToken)
	if err != nil {
		return fmt.Errorf("check existing purchase: %w", err)
	}
	if existing != nil {
		return nil
	}

	product, err := s.shopRepo.GetProductByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	if !product.IsActive {
		return ErrProductNotActive
	}

	switch product.Type {
	case apishop.ProductTypeFactionSet:
		var content apishop.FactionSetContent
		if err := json.Unmarshal(product.Content, &content); err != nil {
			return fmt.Errorf("parse faction set content: %w", err)
		}
		ownedFactions, err := s.ownedFactionRepo.List(ctx, playerID)
		if err != nil {
			return fmt.Errorf("check owned factions: %w", err)
		}
		if slices.Contains(ownedFactions, content.Faction) {
			return ErrAlreadyOwned
		}
	case apishop.ProductTypeCosmetic:
		var content apishop.CosmeticContent
		if err := json.Unmarshal(product.Content, &content); err != nil {
			return fmt.Errorf("parse cosmetic content: %w", err)
		}
		owned, err := s.shopRepo.HasPlayerItem(ctx, playerID, content.ItemType, content.ItemNo)
		if err != nil {
			return fmt.Errorf("check owned item: %w", err)
		}
		if owned {
			return ErrAlreadyOwned
		}
	}

	result, err := verifier.VerifyPurchase(ctx, purchaseToken)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerifyReceipt, err)
	}
	if !result.IsValid {
		return ErrReceiptVerificationFailed
	}

	purchase := &apishop.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   productID,
		PurchasedAt: time.Now(),
	}

	var (
		grantedFaction string
	)

	if err := s.txRunner.RunInTx(ctx, func(ctx context.Context) error {
		switch product.Type {
		case apishop.ProductTypeFactionSet:
			var content apishop.FactionSetContent
			if err := json.Unmarshal(product.Content, &content); err != nil {
				return fmt.Errorf("parse faction set content: %w", err)
			}
			if !slices.Contains(gamedesign.SelectableFactions, content.Faction) {
				return fmt.Errorf("%w: %s", ErrInvalidFaction, content.Faction)
			}
			if err := s.shopRepo.CreatePurchase(ctx, purchase, pf, purchaseToken); err != nil {
				return fmt.Errorf("create purchase: %w", err)
			}
			if err := s.ownedFactionRepo.Add(ctx, playerID, content.Faction); err != nil {
				return fmt.Errorf("add shop-local owned faction: %w", err)
			}
			grantedFaction = content.Faction

		case apishop.ProductTypeCosmetic:
			var content apishop.CosmeticContent
			if err := json.Unmarshal(product.Content, &content); err != nil {
				return fmt.Errorf("parse cosmetic content: %w", err)
			}
			item := &apishop.PlayerItem{
				PlayerID:   playerID,
				ItemType:   content.ItemType,
				ItemNo:     content.ItemNo,
				AcquiredAt: time.Now(),
			}
			if err := s.shopRepo.CreatePurchaseWithItem(ctx, purchase, item, pf, purchaseToken); err != nil {
				return fmt.Errorf("create purchase with item: %w", err)
			}

		default:
			return fmt.Errorf("%w: %s", ErrUnsupportedProductType, product.Type)
		}
		return nil
	}); err != nil {
		return err
	}

	// commit 後に publish する。shop 行 + ローカル read model が永続記録。
	// publish 失敗時は webhook リトライがフローを再駆動する。
	if grantedFaction != "" && s.factionPublisher != nil {
		if err := s.factionPublisher.PublishFactionSelected(ctx, playerID, grantedFaction); err != nil {
			return fmt.Errorf("publish faction-selected: %w", err)
		}
	}

	return nil
}

// Subscribe はサブスクリプション購入フローを実行する。レシート検証・べき等チェック・
// サブスクリプション記録・premium-updated イベント発行を行う。
func (s *ShopService) Subscribe(ctx context.Context, playerID, productID, pf, purchaseToken string) (*time.Time, error) {
	verifier := s.getVerifier(pf)
	if verifier == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, pf)
	}

	existing, err := s.subRepo.FindSubscriptionByToken(ctx, pf, purchaseToken)
	if err != nil {
		return nil, fmt.Errorf("check existing subscription: %w", err)
	}
	if existing != nil {
		return &existing.CurrentPeriodEnd, nil
	}

	product, err := s.shopRepo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if product.Type != apishop.ProductTypeSubscription {
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
	sub := &apishop.Subscription{
		PlayerID:           playerID,
		ProductID:          productID,
		Status:             apishop.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   info.ExpiresAt,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.subRepo.CreateSubscription(ctx, sub, pf, purchaseToken); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	if s.premiumPublisher != nil {
		if err := s.premiumPublisher.PublishPremiumUpdated(ctx, playerID, true, &info.ExpiresAt); err != nil {
			return nil, fmt.Errorf("publish premium-updated: %w", err)
		}
	}

	return &info.ExpiresAt, nil
}

func (s *ShopService) getVerifier(pf string) platform.ReceiptVerifier {
	switch pf {
	case apishop.PlatformIOS:
		return s.appleVerifier
	case apishop.PlatformAndroid:
		return s.googleVerifier
	default:
		return nil
	}
}
