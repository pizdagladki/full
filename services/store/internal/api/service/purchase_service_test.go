package service_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/store/internal/api/repository/mocks"
	svcmocks "github.com/pizdagladki/full/services/store/internal/api/service/mocks"

	"github.com/pizdagladki/full/services/store/internal/api/service"
)

func TestPurchaseService_InitiatePurchase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	distractionProduct := &domain.Product{
		ID: 10, Kind: domain.KindDistraction, PriceCents: 200,
	}
	editProduct := &domain.Product{
		ID: 20, Kind: domain.KindEdit, PriceCents: 500,
	}

	tests := []struct {
		name       string
		userID     int64
		productID  int64
		setupRepo  func(r *repomocks.MockPurchaseRepository)
		setupProv  func(p *svcmocks.MockPaymentProvider)
		wantSecret string
		wantErr    error
	}{
		{
			// criterion: 6 — InitiatePurchase returns client secret for a distraction product
			name:      "success distraction product creates pending purchase returns client secret",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(distractionProduct, nil)
				r.EXPECT().CreatePurchase(ctx, gomock.Any()).Return(int64(1), nil)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().CreatePaymentIntent(ctx, int64(10), 200).Return("cs_test", "pi_test", nil)
			},
			wantSecret: "cs_test",
		},
		{
			// criterion: 6 — InitiatePurchase for edit product that is not yet owned succeeds
			name:      "success edit product not owned creates pending purchase",
			userID:    1,
			productID: 20,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(20)).Return(editProduct, nil)
				r.EXPECT().IsOwned(ctx, int64(1), int64(20)).Return(false, nil)
				r.EXPECT().CreatePurchase(ctx, gomock.Any()).Return(int64(2), nil)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().CreatePaymentIntent(ctx, int64(20), 500).Return("cs_edit", "pi_edit", nil)
			},
			wantSecret: "cs_edit",
		},
		{
			// criterion: 7 — InitiatePurchase returns ErrProductNotFound when product absent
			name:      "product not found returns ErrProductNotFound",
			userID:    1,
			productID: 99,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(99)).Return(nil, domain.ErrProductNotFound)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {},
			wantErr:   domain.ErrProductNotFound,
		},
		{
			// criterion: 8 — InitiatePurchase returns ErrAlreadyOwned for owned edit product
			name:      "edit already owned returns ErrAlreadyOwned",
			userID:    1,
			productID: 20,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(20)).Return(editProduct, nil)
				r.EXPECT().IsOwned(ctx, int64(1), int64(20)).Return(true, nil)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {},
			wantErr:   domain.ErrAlreadyOwned,
		},
		{
			name:      "payment provider error propagates",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(distractionProduct, nil)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().CreatePaymentIntent(ctx, int64(10), 200).Return("", "", errors.New("stripe down"))
			},
			wantErr: errors.New("stripe down"),
		},
		{
			name:      "create purchase error propagates",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(distractionProduct, nil)
				r.EXPECT().CreatePurchase(ctx, gomock.Any()).Return(int64(0), errors.New("db error"))
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().CreatePaymentIntent(ctx, int64(10), 200).Return("cs_x", "pi_x", nil)
			},
			wantErr: errors.New("db error"),
		},
		{
			name:      "is owned check error propagates",
			userID:    1,
			productID: 20,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(20)).Return(editProduct, nil)
				r.EXPECT().IsOwned(ctx, int64(1), int64(20)).Return(false, errors.New("db error"))
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {},
			wantErr:   errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			repo := repomocks.NewMockPurchaseRepository(ctrl)
			prov := svcmocks.NewMockPaymentProvider(ctrl)
			cache := repomocks.NewMockPointsCache(ctrl)

			tt.setupRepo(repo)
			tt.setupProv(prov)

			svc := service.NewPurchaseService(repo, prov, cache, zap.NewNop())
			got, err := svc.InitiatePurchase(ctx, tt.userID, tt.productID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("InitiatePurchase() error = nil, want error")
				}

				if tt.wantErr == domain.ErrProductNotFound && !errors.Is(err, domain.ErrProductNotFound) {
					t.Errorf("InitiatePurchase() error = %v, want ErrProductNotFound", err)
				}

				if tt.wantErr == domain.ErrAlreadyOwned && !errors.Is(err, domain.ErrAlreadyOwned) {
					t.Errorf("InitiatePurchase() error = %v, want ErrAlreadyOwned", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("InitiatePurchase() unexpected error = %v", err)
			}

			if got != tt.wantSecret {
				t.Errorf("InitiatePurchase() clientSecret = %q, want %q", got, tt.wantSecret)
			}
		})
	}
}

func TestPurchaseService_HandleWebhook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	distractionProduct := &domain.Product{
		ID: 10, Kind: domain.KindDistraction, PriceCents: 200,
	}
	editProduct := &domain.Product{
		ID: 20, Kind: domain.KindEdit, PriceCents: 500,
	}
	distractionPurchase := &domain.Purchase{
		ID: 1, UserID: 5, ProductID: 10,
		Provider: domain.ProviderStripe, ProviderRef: "pi_dist",
		AmountCents: 200, Status: domain.PurchaseStatusPending,
	}
	editPurchase := &domain.Purchase{
		ID: 2, UserID: 6, ProductID: 20,
		Provider: domain.ProviderStripe, ProviderRef: "pi_edit",
		AmountCents: 500, Status: domain.PurchaseStatusPending,
	}

	payload := []byte(`{"id":"evt_1"}`)
	sigHeader := "t=1,v1=abc"

	tests := []struct {
		name      string
		setupRepo func(r *repomocks.MockPurchaseRepository)
		setupProv func(p *svcmocks.MockPaymentProvider)
		wantErr   bool
		wantErrIs error
	}{
		{
			// criterion: 9 — HandleWebhook confirms and grants for distraction product
			name: "success distraction confirms and grants inventory increment",
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().WebhookEventExists(ctx, "evt_dist").Return(false, nil)
				r.EXPECT().FindByProviderRef(ctx, "pi_dist").Return(distractionPurchase, nil)
				r.EXPECT().GetProduct(ctx, int64(10)).Return(distractionProduct, nil)
				r.EXPECT().ConfirmAndGrant(ctx, "pi_dist", "evt_dist", domain.KindDistraction, int64(5), int64(10)).Return(nil)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().VerifyWebhook(payload, sigHeader).Return("evt_dist", "pi_dist", true, nil)
			},
		},
		{
			// criterion: 9 — HandleWebhook confirms and grants for edit product
			name: "success edit confirms and grants inventory",
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().WebhookEventExists(ctx, "evt_edit").Return(false, nil)
				r.EXPECT().FindByProviderRef(ctx, "pi_edit").Return(editPurchase, nil)
				r.EXPECT().GetProduct(ctx, int64(20)).Return(editProduct, nil)
				r.EXPECT().ConfirmAndGrant(ctx, "pi_edit", "evt_edit", domain.KindEdit, int64(6), int64(20)).Return(nil)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().VerifyWebhook(payload, sigHeader).Return("evt_edit", "pi_edit", true, nil)
			},
		},
		{
			// criterion: 10 — HandleWebhook returns ErrInvalidWebhook on verify error
			name:      "verify error returns ErrInvalidWebhook",
			setupRepo: func(r *repomocks.MockPurchaseRepository) {},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().VerifyWebhook(payload, sigHeader).Return("", "", false, errors.New("bad sig"))
			},
			wantErr:   true,
			wantErrIs: domain.ErrInvalidWebhook,
		},
		{
			// criterion: 11 — HandleWebhook returns nil for non-succeeded events
			name:      "not succeeded event returns nil",
			setupRepo: func(r *repomocks.MockPurchaseRepository) {},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().VerifyWebhook(payload, sigHeader).Return("evt_other", "", false, nil)
			},
		},
		{
			// criterion: 12 — HandleWebhook is idempotent (no double grant on duplicate event)
			name: "duplicate event is idempotent returns nil",
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().WebhookEventExists(ctx, "evt_dup").Return(true, nil)
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().VerifyWebhook(payload, sigHeader).Return("evt_dup", "pi_dup", true, nil)
			},
		},
		{
			name: "find by provider ref error propagates",
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().WebhookEventExists(ctx, "evt_x").Return(false, nil)
				r.EXPECT().FindByProviderRef(ctx, "pi_x").Return(nil, errors.New("not found"))
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().VerifyWebhook(payload, sigHeader).Return("evt_x", "pi_x", true, nil)
			},
			wantErr: true,
		},
		{
			name: "webhook event check error propagates",
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().WebhookEventExists(ctx, "evt_y").Return(false, errors.New("db error"))
			},
			setupProv: func(p *svcmocks.MockPaymentProvider) {
				p.EXPECT().VerifyWebhook(payload, sigHeader).Return("evt_y", "pi_y", true, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			repo := repomocks.NewMockPurchaseRepository(ctrl)
			prov := svcmocks.NewMockPaymentProvider(ctrl)
			cache := repomocks.NewMockPointsCache(ctrl)

			tt.setupRepo(repo)
			tt.setupProv(prov)

			svc := service.NewPurchaseService(repo, prov, cache, zap.NewNop())
			err := svc.HandleWebhook(ctx, payload, sigHeader)

			if tt.wantErr {
				if err == nil {
					t.Fatal("HandleWebhook() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("HandleWebhook() error = %v, want %v", err, tt.wantErrIs)
				}

				return
			}

			if err != nil {
				t.Fatalf("HandleWebhook() unexpected error = %v", err)
			}
		})
	}
}

func TestPurchaseService_PurchaseWithPoints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	pointsPrice := int64(50)
	distractionProduct := &domain.Product{
		ID: 10, Kind: domain.KindDistraction, PriceCents: 200, PointsPrice: &pointsPrice,
	}
	editProduct := &domain.Product{
		ID: 20, Kind: domain.KindEdit, PriceCents: 500, PointsPrice: &pointsPrice,
	}
	moneyOnlyProduct := &domain.Product{
		ID: 30, Kind: domain.KindDistraction, PriceCents: 100, PointsPrice: nil,
	}

	tests := []struct {
		name        string
		userID      int64
		productID   int64
		setupRepo   func(r *repomocks.MockPurchaseRepository)
		setupCache  func(c *repomocks.MockPointsCache)
		wantBalance int64
		wantErr     error
	}{
		{
			// criterion: 2 — points purchase debits balance and grants inventory in one
			// call, then invalidates the points cache.
			name:      "success distraction product debits points and grants inventory",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(distractionProduct, nil)
				r.EXPECT().PurchaseWithPoints(ctx, int64(1), int64(10), int64(50), domain.KindDistraction).
					Return(int64(450), nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(1)).Return(nil)
			},
			wantBalance: 450,
		},
		{
			// criterion: 2 — edit product not yet owned is charged with points and granted.
			name:      "success edit product not owned debits points and grants inventory",
			userID:    1,
			productID: 20,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(20)).Return(editProduct, nil)
				r.EXPECT().IsOwned(ctx, int64(1), int64(20)).Return(false, nil)
				r.EXPECT().PurchaseWithPoints(ctx, int64(1), int64(20), int64(50), domain.KindEdit).
					Return(int64(450), nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(1)).Return(nil)
			},
			wantBalance: 450,
		},
		{
			// criterion: 3 — insufficient balance propagates ErrInsufficientPoints and
			// nothing is invalidated/written (the repo already wrote nothing in its own tx).
			name:      "insufficient balance returns ErrInsufficientPoints",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(distractionProduct, nil)
				r.EXPECT().PurchaseWithPoints(ctx, int64(1), int64(10), int64(50), domain.KindDistraction).
					Return(int64(0), domain.ErrInsufficientPoints)
			},
			setupCache: func(c *repomocks.MockPointsCache) {},
			wantErr:    domain.ErrInsufficientPoints,
		},
		{
			// criterion: 3 — a money-only product (points_price nil) returns ErrMoneyOnly
			// without ever calling the repo's points-spend path.
			name:      "money only product returns ErrMoneyOnly",
			userID:    1,
			productID: 30,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(30)).Return(moneyOnlyProduct, nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {},
			wantErr:    domain.ErrMoneyOnly,
		},
		{
			// criterion: 5 — an edit already owned is never charged points for again.
			name:      "edit already owned returns ErrAlreadyOwned without charging points",
			userID:    1,
			productID: 20,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(20)).Return(editProduct, nil)
				r.EXPECT().IsOwned(ctx, int64(1), int64(20)).Return(true, nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {},
			wantErr:    domain.ErrAlreadyOwned,
		},
		{
			name:      "product not found returns ErrProductNotFound",
			userID:    1,
			productID: 99,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(99)).Return(nil, domain.ErrProductNotFound)
			},
			setupCache: func(c *repomocks.MockPointsCache) {},
			wantErr:    domain.ErrProductNotFound,
		},
		{
			// cache invalidation failure is logged and swallowed — the purchase still
			// succeeds and the (already-debited) balance is returned.
			name:      "cache invalidate failure does not fail the request",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(distractionProduct, nil)
				r.EXPECT().PurchaseWithPoints(ctx, int64(1), int64(10), int64(50), domain.KindDistraction).
					Return(int64(450), nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(1)).Return(errors.New("redis down"))
			},
			wantBalance: 450,
		},
		{
			name:      "is owned check error propagates",
			userID:    1,
			productID: 20,
			setupRepo: func(r *repomocks.MockPurchaseRepository) {
				r.EXPECT().GetProduct(ctx, int64(20)).Return(editProduct, nil)
				r.EXPECT().IsOwned(ctx, int64(1), int64(20)).Return(false, errors.New("db error"))
			},
			setupCache: func(c *repomocks.MockPointsCache) {},
			wantErr:    errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			repo := repomocks.NewMockPurchaseRepository(ctrl)
			prov := svcmocks.NewMockPaymentProvider(ctrl)
			cache := repomocks.NewMockPointsCache(ctrl)

			tt.setupRepo(repo)
			tt.setupCache(cache)

			svc := service.NewPurchaseService(repo, prov, cache, zap.NewNop())
			got, err := svc.PurchaseWithPoints(ctx, tt.userID, tt.productID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("PurchaseWithPoints() error = nil, want error")
				}

				sentinels := []error{
					domain.ErrProductNotFound, domain.ErrMoneyOnly,
					domain.ErrInsufficientPoints, domain.ErrAlreadyOwned,
				}

				for _, s := range sentinels {
					if errors.Is(tt.wantErr, s) && !errors.Is(err, s) {
						t.Errorf("PurchaseWithPoints() error = %v, want %v", err, s)
					}
				}

				return
			}

			if err != nil {
				t.Fatalf("PurchaseWithPoints() unexpected error = %v", err)
			}

			if got != tt.wantBalance {
				t.Errorf("PurchaseWithPoints() balance = %d, want %d", got, tt.wantBalance)
			}
		})
	}
}
