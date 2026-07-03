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

func TestPointsService_Credit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	amounts := map[string]int64{
		"match_win": 10,
		"level_up":  25,
	}

	tests := []struct {
		name        string
		in          domain.PointsCredit
		setupRepo   func(r *repomocks.MockPointsRepository)
		setupCache  func(c *repomocks.MockPointsCache)
		wantBalance int64
		wantErr     error
	}{
		{
			// criterion: 1, 4 — config-driven amount is resolved by reason (match_win)
			// when no explicit delta is given, and the credit appends+increments via
			// the repository in one call, then the cache is INVALIDATED (not
			// write-through) so the next read repopulates from Postgres.
			name: "config driven amount resolved for reason with no explicit delta",
			in:   domain.PointsCredit{UserID: 1, Reason: "match_win", RefID: "m-1"},
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().Credit(ctx, int64(1), int64(10), "match_win", "m-1").Return(int64(10), true, nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(1)).Return(nil)
			},
			wantBalance: 10,
		},
		{
			// criterion: 4 — a different reason resolves a different config amount
			// (level_up), proving amounts are NOT hardcoded per-reason in Go.
			name: "config driven amount resolved for level_up reason",
			in:   domain.PointsCredit{UserID: 2, Reason: "level_up", RefID: "l-1"},
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().Credit(ctx, int64(2), int64(25), "level_up", "l-1").Return(int64(25), true, nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(2)).Return(nil)
			},
			wantBalance: 25,
		},
		{
			// criterion: 1 — an explicit positive delta overrides the config lookup.
			name: "explicit positive delta overrides config amount",
			in:   domain.PointsCredit{UserID: 3, Reason: "match_win", RefID: "m-2", Delta: 999},
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().Credit(ctx, int64(3), int64(999), "match_win", "m-2").Return(int64(999), true, nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(3)).Return(nil)
			},
			wantBalance: 999,
		},
		{
			// criterion: 2 — a duplicate ref (idempotent hit at the repo layer)
			// returns the existing balance from the repository, and the cache is
			// still invalidated (a no-op if it was already consistent, but safe
			// if a prior write left it stale).
			name: "idempotent duplicate returns existing balance from repo",
			in:   domain.PointsCredit{UserID: 4, Reason: "match_win", RefID: "m-dup"},
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().Credit(ctx, int64(4), int64(10), "match_win", "m-dup").Return(int64(50), false, nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(4)).Return(nil)
			},
			wantBalance: 50,
		},
		{
			// criterion: 4 — empty reason is invalid (400 upstream), regardless of delta.
			name:       "empty reason returns ErrInvalidCredit",
			in:         domain.PointsCredit{UserID: 5, Reason: "", RefID: "x", Delta: 100},
			setupRepo:  func(_ *repomocks.MockPointsRepository) {},
			setupCache: func(_ *repomocks.MockPointsCache) {},
			wantErr:    domain.ErrInvalidCredit,
		},
		{
			// criterion: 4 — non-positive resolved delta (reason not in config, no
			// explicit delta override) is invalid.
			name:       "unknown reason with no config entry resolves to non-positive delta",
			in:         domain.PointsCredit{UserID: 6, Reason: "unknown_reason", RefID: "x"},
			setupRepo:  func(_ *repomocks.MockPointsRepository) {},
			setupCache: func(_ *repomocks.MockPointsCache) {},
			wantErr:    domain.ErrInvalidCredit,
		},
		{
			// criterion: 1, 4 — a non-positive explicit delta does NOT override; it
			// falls back to the config lookup by reason. When that reason has no
			// config entry the resolved delta is 0 (non-positive) -> ErrInvalidCredit.
			name:       "non-positive explicit delta falls back to config, unknown reason invalid",
			in:         domain.PointsCredit{UserID: 7, Reason: "unmapped_reason", RefID: "x", Delta: -5},
			setupRepo:  func(_ *repomocks.MockPointsRepository) {},
			setupCache: func(_ *repomocks.MockPointsCache) {},
			wantErr:    domain.ErrInvalidCredit,
		},
		{
			name: "repo error propagates",
			in:   domain.PointsCredit{UserID: 8, Reason: "match_win", RefID: "m-3"},
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().Credit(ctx, int64(8), int64(10), "match_win", "m-3").
					Return(int64(0), false, errors.New("db error"))
			},
			setupCache: func(_ *repomocks.MockPointsCache) {},
			wantErr:    errors.New("db error"),
		},
		{
			// criterion: cache-invalidation — a cache DELETE (invalidate) failure on
			// credit is logged and swallowed; Postgres remains authoritative and the
			// balance is still returned successfully. This is the scenario FIX 1
			// specifically targets: a transient Redis error on the credit's
			// invalidate must not leave the request failing NOR must it write a
			// possibly-stale value (it simply doesn't write at all).
			name: "cache invalidate failure on credit does not fail the request",
			in:   domain.PointsCredit{UserID: 9, Reason: "match_win", RefID: "m-4"},
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().Credit(ctx, int64(9), int64(10), "match_win", "m-4").Return(int64(10), true, nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().DeleteBalance(ctx, int64(9)).Return(errors.New("redis down"))
			},
			wantBalance: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockPointsRepository(ctrl)
			cache := repomocks.NewMockPointsCache(ctrl)

			tt.setupRepo(repo)
			tt.setupCache(cache)

			svc := service.NewPointsService(repo, cache, amounts, zap.NewNop())
			got, err := svc.Credit(ctx, tt.in)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("Credit() error = nil, want error")
				}

				if errors.Is(tt.wantErr, domain.ErrInvalidCredit) && !errors.Is(err, domain.ErrInvalidCredit) {
					t.Errorf("Credit() error = %v, want ErrInvalidCredit", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Credit() unexpected error = %v", err)
			}

			if got != tt.wantBalance {
				t.Errorf("Credit() balance = %d, want %d", got, tt.wantBalance)
			}
		})
	}
}

func TestPointsService_GetBalance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	amounts := map[string]int64{"match_win": 10}

	tests := []struct {
		name        string
		userID      int64
		setupRepo   func(r *repomocks.MockPointsRepository)
		setupCache  func(c *repomocks.MockPointsCache)
		wantBalance int64
		wantErr     bool
	}{
		{
			// criterion: 3 — cache hit returns the cached value without touching Postgres.
			name:   "cache hit returns cached balance without repo call",
			userID: 1,
			setupRepo: func(_ *repomocks.MockPointsRepository) {
				// no repo call expected on cache hit
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().GetBalance(ctx, int64(1)).Return(int64(55), true, nil)
			},
			wantBalance: 55,
		},
		{
			// criterion: 3 — cache miss falls through to Postgres (source of truth)
			// and repopulates the cache.
			name:   "cache miss falls through to postgres and repopulates cache",
			userID: 2,
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().GetBalance(ctx, int64(2)).Return(int64(30), nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().GetBalance(ctx, int64(2)).Return(int64(0), false, nil)
				c.EXPECT().SetBalance(ctx, int64(2), int64(30)).Return(nil)
			},
			wantBalance: 30,
		},
		{
			// criterion: 3 — a cache READ error (not just a miss) still falls through
			// to Postgres rather than failing the request.
			name:   "cache read error falls through to postgres",
			userID: 3,
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().GetBalance(ctx, int64(3)).Return(int64(12), nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().GetBalance(ctx, int64(3)).Return(int64(0), false, errors.New("redis down"))
				c.EXPECT().SetBalance(ctx, int64(3), int64(12)).Return(nil)
			},
			wantBalance: 12,
		},
		{
			name:   "repo error on cache miss propagates",
			userID: 4,
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().GetBalance(ctx, int64(4)).Return(int64(0), errors.New("db error"))
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().GetBalance(ctx, int64(4)).Return(int64(0), false, nil)
			},
			wantErr: true,
		},
		{
			// criterion: cache-repopulate-failure — a cache SetBalance failure after a
			// Postgres read is logged and swallowed; the balance is still returned.
			name:   "cache repopulate failure after postgres read does not fail request",
			userID: 5,
			setupRepo: func(r *repomocks.MockPointsRepository) {
				r.EXPECT().GetBalance(ctx, int64(5)).Return(int64(7), nil)
			},
			setupCache: func(c *repomocks.MockPointsCache) {
				c.EXPECT().GetBalance(ctx, int64(5)).Return(int64(0), false, nil)
				c.EXPECT().SetBalance(ctx, int64(5), int64(7)).Return(errors.New("redis down"))
			},
			wantBalance: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockPointsRepository(ctrl)
			cache := repomocks.NewMockPointsCache(ctrl)

			tt.setupRepo(repo)
			tt.setupCache(cache)

			svc := service.NewPointsService(repo, cache, amounts, zap.NewNop())
			got, err := svc.GetBalance(ctx, tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetBalance() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GetBalance() unexpected error = %v", err)
			}

			if got != tt.wantBalance {
				t.Errorf("GetBalance() = %d, want %d", got, tt.wantBalance)
			}
		})
	}
}
