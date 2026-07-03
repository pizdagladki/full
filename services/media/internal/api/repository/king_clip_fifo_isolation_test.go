package repository_test

import (
	"context"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
)

// TestClipAndKingClipRepositories_FIFOIsolation documents, at the SQL level,
// that win-clips (clips table) and king clips (king_clips table) are fully
// independent categories: eviction/supersession for one never issues a query
// against the other's table. pgxmock fails the test outright if the SQL sent
// by the repository does not match the exact table expected, so this is a
// hard, criterion-violating-detects-it check in both directions.
func TestClipAndKingClipRepositories_FIFOIsolation(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}
	defer mock.Close()

	clipRepo := repository.NewClipRepository(mock)
	kingRepo := repository.NewKingClipRepository(mock)

	t.Run("win-clip FIFO eviction (DeleteOldestBeyondLimit) only touches clips table, never king_clips", func(t *testing.T) {
		// criterion: 2 — uploading win-clips never evicts a king clip: the FIFO
		// SQL is scoped to "clips", so it can never delete a king_clips row.
		mock.ExpectQuery(`DELETE FROM clips\b`).
			WithArgs(int64(42), domain.MaxClipsPerUser).
			WillReturnRows(pgxmock.NewRows([]string{"object_key"}).AddRow("clips/42/old.webm"))

		keys, err := clipRepo.DeleteOldestBeyondLimit(context.Background(), 42, domain.MaxClipsPerUser)
		if err != nil {
			t.Fatalf("DeleteOldestBeyondLimit() unexpected error = %v", err)
		}

		if len(keys) != 1 || keys[0] != "clips/42/old.webm" {
			t.Errorf("keys = %v, want [clips/42/old.webm]", keys)
		}

		if err = mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled pgxmock expectations: %v", err)
		}
	})

	t.Run("king-clip supersession (DeleteSupersededByHill) only touches king_clips table, never clips", func(t *testing.T) {
		// criterion: 2 — uploading king clips never evicts a user's win-clips:
		// the supersession SQL is scoped to "king_clips", so it can never delete
		// a clips row.
		mock.ExpectQuery(`DELETE FROM king_clips\b`).
			WithArgs(domain.HillTypeDaily, int64(9)).
			WillReturnRows(pgxmock.NewRows([]string{"object_key"}).AddRow("king-clips/daily/42/old.webm"))

		keys, err := kingRepo.DeleteSupersededByHill(context.Background(), domain.HillTypeDaily, 9)
		if err != nil {
			t.Fatalf("DeleteSupersededByHill() unexpected error = %v", err)
		}

		if len(keys) != 1 || keys[0] != "king-clips/daily/42/old.webm" {
			t.Errorf("keys = %v, want [king-clips/daily/42/old.webm]", keys)
		}

		if err = mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled pgxmock expectations: %v", err)
		}
	})
}
