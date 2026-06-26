package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

type purchaseService struct {
	repo     repository.PurchaseRepository
	provider PaymentProvider
	logger   *zap.Logger
}

// NewPurchaseService returns a PurchaseService wired to repo and provider.
func NewPurchaseService(
	repo repository.PurchaseRepository,
	provider PaymentProvider,
	logger *zap.Logger,
) PurchaseService {
	return &purchaseService{repo: repo, provider: provider, logger: logger}
}

// InitiatePurchase creates a Stripe PaymentIntent and stores a pending purchase.
// For edit products it checks ownership and returns ErrAlreadyOwned if owned.
func (s *purchaseService) InitiatePurchase(ctx context.Context, userID, productID int64) (string, error) {
	product, err := s.repo.GetProduct(ctx, productID)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			return "", domain.ErrProductNotFound
		}

		return "", fmt.Errorf("get product: %w", err)
	}

	if product.Kind == domain.KindEdit {
		owned, err := s.repo.IsOwned(ctx, userID, productID)
		if err != nil {
			return "", fmt.Errorf("check ownership: %w", err)
		}

		if owned {
			return "", domain.ErrAlreadyOwned
		}
	}

	clientSecret, paymentIntentID, err := s.provider.CreatePaymentIntent(ctx, productID, product.PriceCents)
	if err != nil {
		return "", fmt.Errorf("create payment intent: %w", err)
	}

	_, err = s.repo.CreatePurchase(ctx, domain.Purchase{
		UserID:      userID,
		ProductID:   productID,
		Provider:    domain.ProviderStripe,
		ProviderRef: paymentIntentID,
		AmountCents: product.PriceCents,
		Status:      domain.PurchaseStatusPending,
	})
	if err != nil {
		return "", fmt.Errorf("create purchase: %w", err)
	}

	return clientSecret, nil
}

// HandleWebhook processes a Stripe webhook event. Non-success events and
// duplicate events (idempotency) are acknowledged without error.
func (s *purchaseService) HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error {
	eventID, paymentIntentID, succeeded, err := s.provider.VerifyWebhook(payload, sigHeader)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidWebhook, err)
	}

	if !succeeded {
		return nil
	}

	exists, err := s.repo.WebhookEventExists(ctx, eventID)
	if err != nil {
		return fmt.Errorf("check webhook event: %w", err)
	}

	if exists {
		return nil
	}

	purchase, err := s.repo.FindByProviderRef(ctx, paymentIntentID)
	if err != nil {
		return fmt.Errorf("find purchase by provider ref: %w", err)
	}

	product, err := s.repo.GetProduct(ctx, purchase.ProductID)
	if err != nil {
		return fmt.Errorf("get product for webhook: %w", err)
	}

	err = s.repo.ConfirmAndGrant(ctx, paymentIntentID, eventID, product.Kind, purchase.UserID, purchase.ProductID)
	if err != nil {
		return fmt.Errorf("confirm and grant: %w", err)
	}

	return nil
}
