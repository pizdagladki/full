package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/signaling/internal/api/domain"
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
)

// blinkReport records the server-stamped receive time of a single blink/face_lost event.
type blinkReport struct {
	userID     int64
	receivedAt time.Time
}

// arbitrationState tracks the blink-arbitration lifecycle for a single room.
type arbitrationState struct {
	firstReport *blinkReport
	decided     bool
	timer       *time.Timer
}

// signalingService implements SignalingService.
type signalingService struct {
	logger    *zap.Logger
	roomRepo  repository.RoomRepository
	now       func() time.Time
	afterFunc func(time.Duration, func()) *time.Timer

	confirmationBuffer time.Duration

	// rooms is the in-process registry: roomID → (userID → Conn).
	// Single-instance only; K8s scale-out would need a shared pub/sub store.
	mu           sync.Mutex
	rooms        map[string]map[int64]Conn
	arbitrations map[string]*arbitrationState
}

// NewSignalingService constructs a SignalingService.
// now and afterFunc are injectable for fake-clock tests; pass time.Now and
// time.AfterFunc in production.
func NewSignalingService(
	logger *zap.Logger,
	roomRepo repository.RoomRepository,
	now func() time.Time,
	afterFunc func(time.Duration, func()) *time.Timer,
	confirmationBuffer time.Duration,
) SignalingService {
	return &signalingService{
		logger:             logger,
		roomRepo:           roomRepo,
		now:                now,
		afterFunc:          afterFunc,
		confirmationBuffer: confirmationBuffer,
		rooms:              make(map[string]map[int64]Conn),
		arbitrations:       make(map[string]*arbitrationState),
	}
}

// Join validates roomID, tries to add the peer to the Redis room, and registers
// the conn in the in-process hub.
func (s *signalingService) Join(ctx context.Context, conn Conn, roomID string) error {
	err := domain.ValidateRoomID(roomID)
	if err != nil {
		return err
	}

	result, err := s.roomRepo.Join(ctx, roomID, conn.UserID())
	if err != nil {
		return err
	}

	if result == repository.JoinResultFull {
		return domain.ErrRoomFull
	}

	// JoinResultJoined or JoinResultAlreadyMember — register in the hub.
	s.mu.Lock()
	if s.rooms[roomID] == nil {
		s.rooms[roomID] = make(map[int64]Conn)
	}

	s.rooms[roomID][conn.UserID()] = conn
	s.mu.Unlock()

	return nil
}

// Relay forwards raw bytes verbatim to every other in-process member of roomID.
// The sender must already be registered (via Join) in the in-process hub.
func (s *signalingService) Relay(_ context.Context, conn Conn, roomID string, raw []byte) error {
	senderID := conn.UserID()

	s.mu.Lock()
	members, ok := s.rooms[roomID]
	if !ok || members[senderID] == nil {
		s.mu.Unlock()

		return domain.ErrNotMember
	}

	// Collect peer conns to send to outside the lock.
	peers := make([]Conn, 0, len(members)-1)

	for uid, c := range members {
		if uid != senderID {
			peers = append(peers, c)
		}
	}

	s.mu.Unlock()

	for _, peer := range peers {
		sendErr := peer.Send(raw)
		if sendErr != nil {
			s.logger.Error("relay send",
				zap.Int64("sender", senderID),
				zap.String("room_id", roomID),
				zap.Error(sendErr),
			)
		}
	}

	return nil
}

// ReportEvent records a blink/face_lost event from conn in roomID, stamped with
// the server receive-time. Client-sent timestamps are ignored for ordering.
// Returns ErrNotMember if conn is not in the room, ErrMatchFinished if an outcome
// was already decided (idempotent).
func (s *signalingService) ReportEvent(_ context.Context, conn Conn, roomID string, _ string) error {
	senderID := conn.UserID()
	recv := s.now()

	s.mu.Lock()

	members, ok := s.rooms[roomID]
	if !ok || members[senderID] == nil {
		s.mu.Unlock()

		return domain.ErrNotMember
	}

	arb := s.getOrCreateArbitration(roomID)

	if arb.decided {
		s.mu.Unlock()

		return domain.ErrMatchFinished
	}

	if arb.firstReport == nil {
		// First report: store it and start the confirmation-buffer timer.
		arb.firstReport = &blinkReport{userID: senderID, receivedAt: recv}
		arb.timer = s.afterFunc(s.confirmationBuffer, func() { s.resolveTimeout(roomID) })
		s.mu.Unlock()

		return nil
	}

	// Second report arrived within the buffer — cancel the timer and decide now.
	if arb.timer != nil {
		arb.timer.Stop()
	}

	// The member with the EARLIER server timestamp is the loser.
	var winnerID, loserID int64
	if recv.Before(arb.firstReport.receivedAt) {
		loserID = senderID
		winnerID = arb.firstReport.userID
	} else {
		loserID = arb.firstReport.userID
		winnerID = senderID
	}

	peers := collectConns(members)
	arb.decided = true
	s.mu.Unlock()

	outcome := domain.OutcomeBytes(winnerID, loserID)
	for _, c := range peers {
		_ = c.Send(outcome)
	}

	return nil
}

// Leave is called on disconnect: optionally announces a forfeit outcome, notifies
// the peer with peer_left, removes the sender from the in-process hub, closes the
// peer, and deletes the room from Redis.
func (s *signalingService) Leave(ctx context.Context, conn Conn, roomID string) {
	if roomID == "" {
		return
	}

	senderID := conn.UserID()

	s.mu.Lock()
	members := s.rooms[roomID]

	// Collect peer conns before cleanup.
	var peers []Conn

	for uid, c := range members {
		if uid != senderID {
			peers = append(peers, c)
		}
	}

	// If the match is not yet decided and there is still a peer in the room,
	// the leaving peer forfeits: announce outcome before peer_left.
	var outcomePayload []byte

	if len(peers) > 0 {
		arb := s.arbitrations[roomID] // may be nil if no blink was ever reported
		decided := arb != nil && arb.decided

		if !decided {
			if arb != nil && arb.timer != nil {
				arb.timer.Stop()
			}

			if arb != nil {
				arb.decided = true
			}

			loserID := senderID
			winnerID := peers[0].UserID()
			outcomePayload = domain.OutcomeBytes(winnerID, loserID)
		}
	}

	// Remove the in-process room entry entirely.
	delete(s.rooms, roomID)
	delete(s.arbitrations, roomID)
	s.mu.Unlock()

	// Send outcome (forfeit) before peer_left so the peer knows why it won.
	if outcomePayload != nil {
		for _, peer := range peers {
			_ = peer.Send(outcomePayload)
		}
	}

	// Notify and close each remaining peer outside the lock.
	peerLeft := domain.PeerLeftBytes()

	for _, peer := range peers {
		sendErr := peer.Send(peerLeft)
		if sendErr != nil {
			s.logger.Error("send peer_left",
				zap.Int64("disconnecting", senderID),
				zap.String("room_id", roomID),
				zap.Error(sendErr),
			)
		}

		peer.Close("peer disconnected")
	}

	// Clean up Redis regardless of whether there was a peer.
	err := s.roomRepo.RemoveRoom(ctx, roomID)
	if err != nil {
		s.logger.Error("remove room from redis",
			zap.String("room_id", roomID),
			zap.Error(err),
		)
	}
}

// resolveTimeout is called by the timer when only one member reported within the
// confirmation buffer. The first reporter is the loser; the other member wins.
func (s *signalingService) resolveTimeout(roomID string) {
	s.mu.Lock()

	arb, ok := s.arbitrations[roomID]
	if !ok || arb.decided || arb.firstReport == nil {
		s.mu.Unlock()

		return
	}

	loserID := arb.firstReport.userID

	var winnerID int64

	for uid := range s.rooms[roomID] {
		if uid != loserID {
			winnerID = uid
		}
	}

	peers := collectConns(s.rooms[roomID])
	arb.decided = true
	s.mu.Unlock()

	outcome := domain.OutcomeBytes(winnerID, loserID)
	for _, c := range peers {
		_ = c.Send(outcome)
	}
}

// getOrCreateArbitration returns the arbitrationState for roomID, creating it if absent.
// Must be called with s.mu held.
func (s *signalingService) getOrCreateArbitration(roomID string) *arbitrationState {
	arb, ok := s.arbitrations[roomID]
	if !ok {
		arb = &arbitrationState{}
		s.arbitrations[roomID] = arb
	}

	return arb
}

// collectConns returns all Conn values from a room member map.
func collectConns(members map[int64]Conn) []Conn {
	conns := make([]Conn, 0, len(members))
	for _, c := range members {
		conns = append(conns, c)
	}

	return conns
}
