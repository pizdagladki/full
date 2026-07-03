package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/pizdagladki/full/services/signaling/internal/api/service"
)

func (a *App) initServices() {
	ratingsClient := service.NewHTTPRatingsClient(a.cfg.RatingsBaseURL, &http.Client{Timeout: 10 * time.Second})
	a.signalingSvc = service.NewSignalingService(
		a.logger,
		a.roomRepo,
		time.Now,
		time.AfterFunc,
		a.cfg.Signaling.ConfirmationBuffer,
		ratingsClient,
		a.roomCodeRepo,
		generateRoomID,
	)
}

// roomIDBytes is the number of random bytes used to generate a private
// room_id (16 hex chars), well within domain.ValidateRoomID's 128-char bound.
const roomIDBytes = 8

// generateRoomID returns a fresh cryptographically random room_id (16 hex
// characters) for CreateRoom, or an error if the OS entropy source fails.
func generateRoomID() (string, error) {
	b := make([]byte, roomIDBytes)

	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("read random bytes for room id: %w", err)
	}

	return hex.EncodeToString(b), nil
}
