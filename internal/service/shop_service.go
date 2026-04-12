package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/constants"
	"github.com/kenyamaneko/overload-party-shop/internal/model"
	"github.com/kenyamaneko/overload-party-shop/internal/platform"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// CardLister は shop が faction-set カード列挙に必要とする card サービスの狭い
// スライス。商品カタログメタデータ層でのみ使用。カード付与には使用しない。
type CardLister interface {
	ListAllCards(ctx context.Context) ([]*apishop.CardView, error)
}

func isSelectableFaction(faction string) bool {
	for _, f := range constants.SelectableFactions {
		if f == faction {
			return true
		}
	}
	return false
}

var normalizeFactionMap = map[string]string{
	"she":     constants.FactionSHE,
	"tenki":   constants.FactionTenki,
	"sugar":   constants.FactionSugar,
	"tuners":  constants.FactionTuners,
	"neutral": constants.FactionNeutral,
}

func normalizeFaction(s string) (string, bool) {
	if v, ok := normalizeFactionMap[strings.ToLower(s)]; ok {
		return v, true
	}
	return s, false
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
	ownedFactionSet := make(map[string]bool, len(ownedFactions))
	for _, f := range ownedFactions {
		ownedFactionSet[f] = true
	}

	activeSub, err := s.subRepo.GetActiveSubscription(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
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
			owned = ownedFactionSet[content.Faction]
		case apishop.ProductTypeSubscription:
			owned = activeSub != nil
		// cosmetic 等はユニーク所有ではない — 常に購入可能として表示
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
	existing, err := s.shopRepo.FindPurchaseByToken(ctx, playerID, purchaseToken)
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
		for _, f := range ownedFactions {
			if f == content.Faction {
				return ErrAlreadyOwned
			}
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

	verifier := s.getVerifier(pf)
	if verifier == nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedPlatform, pf)
	}
	result, err := verifier.VerifyPurchase(ctx, purchaseToken)
	if err != nil {
		return fmt.Errorf("verify receipt: %w", err)
	}
	if !result.IsValid {
		return ErrReceiptVerificationFailed
	}

	purchase := &apishop.OneTimePurchase{
		PlayerID:      playerID,
		ProductID:     productID,
		Platform:      pf,
		PurchaseToken: purchaseToken,
		PurchasedAt:   time.Now(),
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
			if !isSelectableFaction(content.Faction) {
				return fmt.Errorf("%w: %s", ErrInvalidFaction, content.Faction)
			}
			if err := s.shopRepo.CreatePurchase(ctx, purchase); err != nil {
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
			if err := s.shopRepo.CreatePurchaseWithItem(ctx, purchase, item); err != nil {
				return fmt.Errorf("create purchase with item: %w", err)
			}

		default:
			return fmt.Errorf("unsupported product type for purchase: %s", product.Type)
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
	existing, err := s.subRepo.FindSubscriptionByToken(ctx, purchaseToken)
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

	verifier := s.getVerifier(pf)
	if verifier == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, pf)
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
		Platform:           pf,
		PurchaseToken:      purchaseToken,
		Status:             apishop.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   info.ExpiresAt,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.subRepo.CreateSubscription(ctx, sub); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	if s.premiumPublisher != nil {
		if err := s.premiumPublisher.PublishPremiumUpdated(ctx, playerID, true, &info.ExpiresAt); err != nil {
			return nil, fmt.Errorf("publish premium-updated: %w", err)
		}
	}

	return &info.ExpiresAt, nil
}

func (s *ShopService) buildFactionCards(ctx context.Context, playerID string, faction string) ([]*model.PlayerCard, error) {
	all, err := s.cardLister.ListAllCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	var cards []*model.PlayerCard
	for _, card := range all {
		if card.Faction != faction || !card.IsActive {
			continue
		}
		cards = append(cards, &model.PlayerCard{
			PlayerID: playerID,
			CardID:   card.CardID,
			ArtNo:    0,
			Count:    3,
		})
	}
	return cards, nil
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
