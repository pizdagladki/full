package service_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/store/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/store/internal/api/service"
)

func TestInventoryService_ListInventory(t *testing.T) {
	t.Parallel()

	ownedItems := []domain.InventoryItem{
		{ProductID: 1, Quantity: 3},
		{ProductID: 5, Quantity: 1},
	}

	tests := []struct {
		name      string
		userID    int64
		setupRepo func(m *repomocks.MockInventoryRepository)
		wantLen   int
		wantErr   bool
		wantItems []domain.InventoryItem // non-nil: assert product_id and quantity by value (criterion: 2)
	}{
		{
			// criterion: 2 — "owned items returned for user" asserts exact product_id and quantity values
			// passed through from the repository to the caller unchanged.
			name:   "owned items returned for user",
			userID: 42,
			setupRepo: func(m *repomocks.MockInventoryRepository) {
				m.EXPECT().ListByUser(gomock.Any(), int64(42)).
					Return(ownedItems, nil)
			},
			wantLen: 2,
			wantItems: []domain.InventoryItem{
				{ProductID: 1, Quantity: 3},
				{ProductID: 5, Quantity: 1},
			},
		},
		{
			name:   "empty inventory returns empty slice",
			userID: 99,
			setupRepo: func(m *repomocks.MockInventoryRepository) {
				m.EXPECT().ListByUser(gomock.Any(), int64(99)).
					Return([]domain.InventoryItem{}, nil)
			},
			wantLen: 0,
		},
		{
			name:   "repo error is propagated",
			userID: 1,
			setupRepo: func(m *repomocks.MockInventoryRepository) {
				m.EXPECT().ListByUser(gomock.Any(), int64(1)).
					Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockInventoryRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewInventoryService(repoMock)

			got, err := svc.ListInventory(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ListInventory() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ListInventory() unexpected error = %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("len(items) = %d, want %d", len(got), tt.wantLen)
			}

			// criterion: 2 — assert exact product_id and quantity for each item.
			for i, want := range tt.wantItems {
				if i >= len(got) {
					t.Errorf("got[%d] missing, have only %d items", i, len(got))
					break
				}

				item := got[i]

				if item.ProductID != want.ProductID {
					t.Errorf("items[%d].ProductID = %d, want %d", i, item.ProductID, want.ProductID)
				}

				if item.Quantity != want.Quantity {
					t.Errorf("items[%d].Quantity = %d, want %d", i, item.Quantity, want.Quantity)
				}
			}
		})
	}
}
