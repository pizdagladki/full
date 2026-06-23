package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
	"github.com/pizdagladki/full/services/matchmaking/internal/api/repository"
)

// realClock is the production Clock implementation.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock is the production clock.
var RealClock Clock = realClock{}

// uuidRoomIDGen generates room ids using github.com/google/uuid.
type uuidRoomIDGen struct{}

func (uuidRoomIDGen) NewRoomID() string {
	return newUUID()
}

// UUIDRoomIDGenerator is the production room-id generator.
var UUIDRoomIDGenerator RoomIDGenerator = uuidRoomIDGen{}

// matchmakingService implements MatchmakingService.
type matchmakingService struct {
	logger        *zap.Logger
	queueRepo     repository.QueueRepository
	clock         Clock
	roomIDGen     RoomIDGenerator
	levelDist     int
	fallbackAfter time.Duration

	mu    sync.Mutex
	conns map[int64]Conn   // userID → active connection; single-instance only — K8s scale-out would need a shared store
	modes map[int64]string // userID → mode (needed for Leave cleanup)
}

// NewMatchmakingService constructs the matchmaking service.
func NewMatchmakingService(
	logger *zap.Logger,
	queueRepo repository.QueueRepository,
	clock Clock,
	roomIDGen RoomIDGenerator,
	levelDist int,
	fallbackAfter time.Duration,
) MatchmakingService {
	return &matchmakingService{
		logger:        logger,
		queueRepo:     queueRepo,
		clock:         clock,
		roomIDGen:     roomIDGen,
		levelDist:     levelDist,
		fallbackAfter: fallbackAfter,
		conns:         make(map[int64]Conn),
		modes:         make(map[int64]string),
	}
}

// Join validates the join request, enqueues the player, and registers the conn.
func (s *matchmakingService) Join(ctx context.Context, conn Conn, player domain.Player) error {
	err := domain.ValidateJoin(player.Mode, player.Level)
	if err != nil {
		return err
	}

	enqErr := s.queueRepo.Enqueue(ctx, player)
	if enqErr != nil {
		return enqErr
	}

	s.mu.Lock()
	s.conns[player.UserID] = conn
	s.modes[player.UserID] = player.Mode
	s.mu.Unlock()

	return nil
}

// Leave removes the player from the queue and deregisters the connection.
func (s *matchmakingService) Leave(ctx context.Context, userID int64, mode string) {
	s.mu.Lock()
	delete(s.conns, userID)
	delete(s.modes, userID)
	s.mu.Unlock()

	_, removeErr := s.queueRepo.Remove(ctx, mode, userID)
	if removeErr != nil {
		s.logger.Error("remove from queue on leave", zap.Int64("user_id", userID), zap.Error(removeErr))
	}
}

// Tick drives one pairing cycle for all currently active modes. It is called
// by the matcher worker periodically. The single-goroutine caller ensures
// serial execution — no Go-level double-pairing.
func (s *matchmakingService) Tick(ctx context.Context) {
	modes := s.activeModes()

	for _, mode := range modes {
		s.tickMode(ctx, mode)
	}
}

// activeModes returns a deduplicated list of modes that currently have
// registered connections.
func (s *matchmakingService) activeModes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[string]struct{}, len(s.modes))
	modes := make([]string, 0, len(s.modes))

	for _, mode := range s.modes {
		if _, ok := seen[mode]; !ok {
			seen[mode] = struct{}{}
			modes = append(modes, mode)
		}
	}

	return modes
}

// tickMode runs one pairing pass for a single mode.
func (s *matchmakingService) tickMode(ctx context.Context, mode string) {
	waiting, err := s.queueRepo.ListWaiting(ctx, mode)
	if err != nil {
		s.logger.Error("list waiting", zap.String("mode", mode), zap.Error(err))

		return
	}

	now := s.clock.Now()
	paired := make(map[int64]struct{})

	for i := range waiting {
		candidate := waiting[i]
		if _, alreadyPaired := paired[candidate.UserID]; alreadyPaired {
			continue
		}

		// Build the slice of still-available opponents.
		available := make([]domain.Player, 0, len(waiting))

		for j := range waiting {
			opp := waiting[j]
			if opp.UserID == candidate.UserID {
				continue
			}
			if _, alreadyPaired := paired[opp.UserID]; alreadyPaired {
				continue
			}

			available = append(available, opp)
		}

		if len(available) == 0 {
			continue
		}

		// In-distance match first.
		opp := domain.NearestWithinDistance(candidate, available, s.levelDist)

		// Fallback: after the deadline, pair with nearest regardless.
		if opp == nil && domain.PastFallbackDeadline(now, candidate.EnqueuedAt, s.fallbackAfter) {
			opp = domain.NearestRegardless(candidate, available)
		}

		if opp == nil {
			continue
		}

		s.tryPair(ctx, candidate, *opp, paired)
	}

	// Count how many players from this tick were not paired away.
	stillWaiting := 0

	for i := range waiting {
		if _, wasPaired := paired[waiting[i].UserID]; !wasPaired {
			stillWaiting++
		}
	}

	// Refresh the backstop TTL while live waiters remain so a connected
	// solo searcher is never silently evicted from Redis by the crash-orphan
	// TTL. A mode with no remaining waiters needs no refresh — its key either
	// no longer exists or will expire naturally.
	if stillWaiting > 0 {
		refreshErr := s.queueRepo.Refresh(ctx, mode)
		if refreshErr != nil {
			s.logger.Error("refresh queue ttl", zap.String("mode", mode), zap.Error(refreshErr))
		}
	}
}

// tryPair atomically claims the pair via the repo and notifies both conns.
func (s *matchmakingService) tryPair(
	ctx context.Context,
	a, b domain.Player,
	paired map[int64]struct{},
) {
	ok, err := s.queueRepo.Pair(ctx, a, b)
	if err != nil {
		s.logger.Error("pair attempt", zap.Error(err))

		return
	}

	if !ok {
		// Lost the race — another goroutine already claimed one of the players.
		return
	}

	roomID := s.roomIDGen.NewRoomID()
	paired[a.UserID] = struct{}{}
	paired[b.UserID] = struct{}{}

	s.mu.Lock()
	connA := s.conns[a.UserID]
	connB := s.conns[b.UserID]
	delete(s.conns, a.UserID)
	delete(s.conns, b.UserID)
	delete(s.modes, a.UserID)
	delete(s.modes, b.UserID)
	s.mu.Unlock()

	if connA != nil {
		sendErr := connA.Send(domain.MatchedMessage{
			Type:     "matched",
			RoomID:   roomID,
			Opponent: b.UserID,
		})
		if sendErr != nil {
			// Close the peer so it reconnects and can rejoin the queue.
			s.logger.Error("send match to a", zap.Error(sendErr))
			connA.Close("send error: reconnect and rejoin")
		}
	}

	if connB != nil {
		sendErr := connB.Send(domain.MatchedMessage{
			Type:     "matched",
			RoomID:   roomID,
			Opponent: a.UserID,
		})
		if sendErr != nil {
			// Close the peer so it reconnects and can rejoin the queue.
			s.logger.Error("send match to b", zap.Error(sendErr))
			connB.Close("send error: reconnect and rejoin")
		}
	}
}
