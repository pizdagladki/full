package service

import (
	"context"
	"fmt"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

type hillService struct {
	repo repository.HillRepository
}

// NewHillService returns a HillService wired to repo.
func NewHillService(repo repository.HillRepository) HillService {
	return &hillService{repo: repo}
}

func (s *hillService) CurrentKing(ctx context.Context, hillType string) (domain.KingReign, error) {
	ht, err := domain.ParseHillType(hillType)
	if err != nil {
		return domain.KingReign{}, err
	}

	king, err := s.repo.CurrentKing(ctx, ht)
	if err != nil {
		return domain.KingReign{}, err
	}

	return *king, nil
}

func (s *hillService) Challenge(
	ctx context.Context, hillType string, userID int64, survivedMs int, newClipID string,
) (domain.ChallengeOutcome, error) {
	ht, err := domain.ParseHillType(hillType)
	if err != nil {
		return domain.ChallengeOutcome{}, err
	}

	outcome, err := s.repo.Challenge(ctx, ht, userID, survivedMs, newClipID)
	if err != nil {
		return domain.ChallengeOutcome{}, fmt.Errorf("challenge: %w", err)
	}

	return outcome, nil
}
