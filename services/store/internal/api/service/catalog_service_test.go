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

func ptrStr(s string) *string { return &s }

func TestCatalogService_ListCatalog(t *testing.T) {
	t.Parallel()

	tier1 := 1
	products := []domain.Product{
		{ID: 1, Kind: "distraction", Tier: &tier1, Name: "Spinner", PriceCents: 0, IsFree: true},
		{ID: 2, Kind: "edit", Name: "Blur", PriceCents: 100, IsFree: false},
	}

	tests := []struct {
		name      string
		kind      *string
		setupRepo func(m *repomocks.MockCatalogRepository)
		wantLen   int
		wantErr   error
	}{
		{
			name: "nil kind lists all products",
			kind: nil,
			setupRepo: func(m *repomocks.MockCatalogRepository) {
				m.EXPECT().ListProducts(gomock.Any(), (*string)(nil)).
					Return(products, nil)
			},
			wantLen: 2,
		},
		{
			name: "valid kind filter is forwarded to repo",
			kind: ptrStr("distraction"),
			setupRepo: func(m *repomocks.MockCatalogRepository) {
				m.EXPECT().ListProducts(gomock.Any(), ptrStr("distraction")).
					Return(products[:1], nil)
			},
			wantLen: 1,
		},
		{
			name:      "invalid kind returns ErrInvalidKind without hitting repo",
			kind:      ptrStr("invalid"),
			setupRepo: func(_ *repomocks.MockCatalogRepository) {},
			wantErr:   domain.ErrInvalidKind,
		},
		{
			name: "repo error is propagated",
			kind: nil,
			setupRepo: func(m *repomocks.MockCatalogRepository) {
				m.EXPECT().ListProducts(gomock.Any(), (*string)(nil)).
					Return(nil, errors.New("db error"))
			},
			wantErr: errors.New("db error"),
		},
		{
			name: "empty result returns empty slice",
			kind: nil,
			setupRepo: func(m *repomocks.MockCatalogRepository) {
				m.EXPECT().ListProducts(gomock.Any(), (*string)(nil)).
					Return([]domain.Product{}, nil)
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockCatalogRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewCatalogService(repoMock)

			got, err := svc.ListCatalog(context.Background(), tt.kind)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ListCatalog() error = nil, want %v", tt.wantErr)
				}

				if errors.Is(tt.wantErr, domain.ErrInvalidKind) && !errors.Is(err, domain.ErrInvalidKind) {
					t.Errorf("ListCatalog() error = %v, want ErrInvalidKind", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ListCatalog() unexpected error = %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("len(products) = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
