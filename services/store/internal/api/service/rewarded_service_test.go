package service_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/store/internal/api/repository/mocks"

	"github.com/pizdagladki/full/services/store/internal/api/service"
)

func TestRewardedService_GrantFreeDistraction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	freeDistraction := &domain.Product{ID: 10, Kind: domain.KindDistraction, IsFree: true}
	paidDistraction := &domain.Product{ID: 11, Kind: domain.KindDistraction, IsFree: false}
	freeEdit := &domain.Product{ID: 12, Kind: domain.KindEdit, IsFree: true}

	tests := []struct {
		name        string
		userID      int64
		productID   int64
		setupRepo   func(r *repomocks.MockRewardedRepository)
		setupLim    func(l *repomocks.MockRewardedRateLimiter)
		wantQty     int
		wantErr     error
		wantErrText string // for non-sentinel errors
	}{
		{
			// criterion: 1 — an eligible free distraction is granted and the new
			// inventory quantity is returned.
			name:      "eligible free distraction grants and returns quantity",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(freeDistraction, nil)
				r.EXPECT().GrantFreeDistraction(ctx, int64(1), int64(10)).Return(2, nil)
			},
			setupLim: func(l *repomocks.MockRewardedRateLimiter) {
				l.EXPECT().Allow(ctx, int64(1)).Return(true, nil)
			},
			wantQty: 2,
		},
		{
			// criterion: 4 — an unknown product returns ErrProductNotFound (404 at
			// the handler) and nothing else is called.
			name:      "unknown product returns ErrProductNotFound",
			userID:    1,
			productID: 99,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(99)).Return(nil, domain.ErrProductNotFound)
			},
			setupLim: func(l *repomocks.MockRewardedRateLimiter) {},
			wantErr:  domain.ErrProductNotFound,
		},
		{
			// criterion: 2 — a paid distraction (is_free=false) is rejected with
			// ErrNotGrantable and GrantFreeDistraction is never called.
			name:      "paid distraction rejected as not grantable",
			userID:    1,
			productID: 11,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(11)).Return(paidDistraction, nil)
			},
			setupLim: func(l *repomocks.MockRewardedRateLimiter) {},
			wantErr:  domain.ErrNotGrantable,
		},
		{
			// criterion: 2 — an edit product (kind=edit) is rejected with
			// ErrNotGrantable even when marked free, and GrantFreeDistraction is
			// never called.
			name:      "edit product rejected as not grantable",
			userID:    1,
			productID: 12,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(12)).Return(freeEdit, nil)
			},
			setupLim: func(l *repomocks.MockRewardedRateLimiter) {},
			wantErr:  domain.ErrNotGrantable,
		},
		{
			// criterion: 3 — the rate limiter denies the grant, returns
			// ErrRateLimited, and GrantFreeDistraction is never called.
			name:      "rate limited returns ErrRateLimited without granting",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(freeDistraction, nil)
			},
			setupLim: func(l *repomocks.MockRewardedRateLimiter) {
				l.EXPECT().Allow(ctx, int64(1)).Return(false, nil)
			},
			wantErr: domain.ErrRateLimited,
		},
		{
			name:      "get product error propagates",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(nil, errors.New("db down"))
			},
			setupLim:    func(l *repomocks.MockRewardedRateLimiter) {},
			wantErrText: "db down",
		},
		{
			name:      "rate limiter error propagates",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(freeDistraction, nil)
			},
			setupLim: func(l *repomocks.MockRewardedRateLimiter) {
				l.EXPECT().Allow(ctx, int64(1)).Return(false, errors.New("redis down"))
			},
			wantErrText: "redis down",
		},
		{
			name:      "grant error propagates",
			userID:    1,
			productID: 10,
			setupRepo: func(r *repomocks.MockRewardedRepository) {
				r.EXPECT().GetProduct(ctx, int64(10)).Return(freeDistraction, nil)
				r.EXPECT().GrantFreeDistraction(ctx, int64(1), int64(10)).Return(0, errors.New("db down"))
			},
			setupLim: func(l *repomocks.MockRewardedRateLimiter) {
				l.EXPECT().Allow(ctx, int64(1)).Return(true, nil)
			},
			wantErrText: "db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			repo := repomocks.NewMockRewardedRepository(ctrl)
			lim := repomocks.NewMockRewardedRateLimiter(ctrl)

			tt.setupRepo(repo)
			tt.setupLim(lim)

			svc := service.NewRewardedService(repo, lim, zap.NewNop())
			got, err := svc.GrantFreeDistraction(ctx, tt.userID, tt.productID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("GrantFreeDistraction() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if tt.wantErrText != "" {
				if err == nil {
					t.Fatal("GrantFreeDistraction() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GrantFreeDistraction() unexpected error = %v", err)
			}

			if got != tt.wantQty {
				t.Errorf("GrantFreeDistraction() quantity = %d, want %d", got, tt.wantQty)
			}
		})
	}
}
