package service

import (
	"context"
	"errors"
	"fmt"
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
	logger       *zap.Logger
	roomRepo     repository.RoomRepository
	roomCodeRepo repository.RoomCodeRepository
	now          func() time.Time
	afterFunc    func(time.Duration, func()) *time.Timer
	// genRoomID generates a fresh room_id for CreateRoom (injectable for
	// deterministic tests; production passes a crypto/rand hex generator).
	genRoomID func() (string, error)

	confirmationBuffer time.Duration
	ratingsClient      RatingsClient

	// rooms is the in-process registry: roomID → (userID → Conn).
	// Single-instance only; K8s scale-out would need a shared pub/sub store.
	mu           sync.Mutex
	rooms        map[string]map[int64]Conn
	arbitrations map[string]*arbitrationState
	// roomModes stores the mode for each room (set on first join).
	roomModes map[string]string
	// battleStart stores the time when a room became full (2 members).
	battleStart map[string]time.Time
	// roomCodes stores the invite code for each private room created via
	// CreateRoom, keyed by roomID, so Leave can clean it up on creator
	// disconnect (before or after a second peer joins).
	roomCodes map[string]string
}

// NewSignalingService constructs a SignalingService.
// now and afterFunc are injectable for fake-clock tests; pass time.Now and
// time.AfterFunc in production. genRoomID generates fresh room ids for
// CreateRoom; production passes a crypto/rand hex generator.
func NewSignalingService(
	logger *zap.Logger,
	roomRepo repository.RoomRepository,
	now func() time.Time,
	afterFunc func(time.Duration, func()) *time.Timer,
	confirmationBuffer time.Duration,
	ratingsClient RatingsClient,
	roomCodeRepo repository.RoomCodeRepository,
	genRoomID func() (string, error),
) SignalingService {
	return &signalingService{
		logger:             logger,
		roomRepo:           roomRepo,
		roomCodeRepo:       roomCodeRepo,
		now:                now,
		afterFunc:          afterFunc,
		genRoomID:          genRoomID,
		confirmationBuffer: confirmationBuffer,
		ratingsClient:      ratingsClient,
		rooms:              make(map[string]map[int64]Conn),
		arbitrations:       make(map[string]*arbitrationState),
		roomModes:          make(map[string]string),
		battleStart:        make(map[string]time.Time),
		roomCodes:          make(map[string]string),
	}
}

// Join validates roomID, tries to add the peer to the Redis room, and registers
// the conn in the in-process hub. mode is stored on first join.
func (s *signalingService) Join(ctx context.Context, conn Conn, roomID string, mode string) error {
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
	s.registerConn(roomID, mode, conn)

	return nil
}

// CreateRoom generates a fresh room_id, registers the caller as the first
// member of a new UNRANKED private room, mints an invite code mapped to it,
// and returns both. On any failure it best-effort rolls back partial state.
func (s *signalingService) CreateRoom(ctx context.Context, conn Conn) (string, string, error) {
	roomID, err := s.genRoomID()
	if err != nil {
		return "", "", fmt.Errorf("generate room id: %w", err)
	}

	result, err := s.roomRepo.Join(ctx, roomID, conn.UserID())
	if err != nil {
		return "", "", fmt.Errorf("join redis room %q: %w", roomID, err)
	}

	// A freshly generated room_id must always be admitted (never full).
	if result == repository.JoinResultFull {
		return "", "", domain.ErrRoomFull
	}

	s.registerConn(roomID, domain.ModeUnranked, conn)

	code, err := s.roomCodeRepo.CreateCode(ctx, roomID)
	if err != nil {
		// Roll back the hub + Redis room so a failed create leaves no residue.
		s.mu.Lock()
		delete(s.rooms, roomID)
		delete(s.roomModes, roomID)
		delete(s.battleStart, roomID)
		s.mu.Unlock()

		removeErr := s.roomRepo.RemoveRoom(ctx, roomID)
		if removeErr != nil {
			s.logger.Error("rollback remove room after code creation failure",
				zap.String("room_id", roomID),
				zap.Error(removeErr),
			)
		}

		return "", "", fmt.Errorf("create invite code for room %q: %w", roomID, err)
	}

	s.mu.Lock()
	s.roomCodes[roomID] = code
	s.mu.Unlock()

	return roomID, code, nil
}

// JoinByCode resolves an invite code to its room_id and joins the caller as
// the second member. The room stays unranked (set by the creator via
// CreateRoom). Returns domain.ErrInvalidCode for an unknown/expired code and
// domain.ErrRoomFull when the room already has two members.
func (s *signalingService) JoinByCode(ctx context.Context, conn Conn, code string) (string, error) {
	roomID, err := s.roomCodeRepo.ResolveCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrCodeNotFound) {
			return "", domain.ErrInvalidCode
		}

		return "", fmt.Errorf("resolve invite code %q: %w", code, err)
	}

	result, err := s.roomRepo.Join(ctx, roomID, conn.UserID())
	if err != nil {
		return "", fmt.Errorf("join redis room %q: %w", roomID, err)
	}

	if result == repository.JoinResultFull {
		return "", domain.ErrRoomFull
	}

	// mode is whatever the creator set (domain.ModeUnranked) — registerConn
	// only stores it on the room's first registration, so the value passed
	// here is ignored when the room already exists in the hub.
	s.registerConn(roomID, domain.ModeUnranked, conn)

	return roomID, nil
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
		//nolint:contextcheck // timer callback fires without a request context
		arb.timer = s.afterFunc(s.confirmationBuffer, func() { s.resolveTimeout(roomID) })
		s.mu.Unlock()

		return nil
	}

	// Guard: same member re-reporting (e.g. EAR fires on multiple frames). Keep waiting.
	if senderID == arb.firstReport.userID {
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

	s.fireRatingsCall(roomID, winnerID, loserID) //nolint:contextcheck

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

	var winnerID, loserID int64
	var forfeitMode string
	var forfeitStart time.Time
	shouldFireRatings := false

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

			loserID = senderID
			winnerID = peers[0].UserID()
			outcomePayload = domain.OutcomeBytes(winnerID, loserID)
			// Capture mode and start before deleting maps.
			forfeitMode = s.roomModes[roomID]
			forfeitStart = s.battleStart[roomID]
			shouldFireRatings = true
		}
	}

	// Remove the in-process room entry entirely.
	delete(s.rooms, roomID)
	delete(s.arbitrations, roomID)
	delete(s.roomModes, roomID)
	delete(s.battleStart, roomID)

	// Capture the invite code (if this was a private room) for cleanup below;
	// the TTL is the backstop if this fails.
	code, hadCode := s.roomCodes[roomID]
	delete(s.roomCodes, roomID)

	s.mu.Unlock()

	if hadCode {
		removeErr := s.roomCodeRepo.RemoveCode(ctx, code)
		if removeErr != nil {
			s.logger.Error("remove invite code on leave",
				zap.String("room_id", roomID),
				zap.Error(removeErr),
			)
		}
	}

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

	// Fire ratings call after releasing the lock — using captured values.
	if shouldFireRatings && forfeitMode == domain.ModeRanked {
		durationMs := s.now().Sub(forfeitStart).Milliseconds()
		req := ApplyResultRequest{
			WinnerID:   winnerID,
			LoserID:    loserID,
			Mode:       forfeitMode,
			DurationMs: durationMs,
		}
		go func() { //nolint:contextcheck,gosec // G118: fire-and-forget; background ctx is intentional
			rCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := s.ratingsClient.ApplyResult(rCtx, req)
			if err != nil {
				s.logger.Error("ratings ApplyResult failed",
					zap.String("room_id", roomID),
					zap.Int64("winner_id", winnerID),
					zap.Int64("loser_id", loserID),
					zap.Error(err),
				)
			}
		}()
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

// registerConn registers conn as a member of roomID in the in-process hub.
// mode is stored the first time a room is registered (first join / CreateRoom)
// and ignored on subsequent calls for the same room. battleStart is recorded
// the moment the room reaches its second member. Shared by Join, CreateRoom,
// and JoinByCode so the hub-registration/battleStart logic never forks.
func (s *signalingService) registerConn(roomID string, mode string, conn Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rooms[roomID] == nil {
		s.rooms[roomID] = make(map[int64]Conn)
		// Store mode on first join.
		s.roomModes[roomID] = mode
	}

	s.rooms[roomID][conn.UserID()] = conn

	// Record battle start time when the room becomes full (2 members).
	if len(s.rooms[roomID]) == 2 {
		if _, ok := s.battleStart[roomID]; !ok {
			s.battleStart[roomID] = s.now()
		}
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

	s.fireRatingsCall(roomID, winnerID, loserID) //nolint:contextcheck // timer callback has no context
}

// fireRatingsCall reads the room mode from s.roomModes and fires an async
// ApplyResult call if the room is ranked. Must be called AFTER releasing s.mu.
func (s *signalingService) fireRatingsCall(roomID string, winnerID, loserID int64) {
	s.mu.Lock()
	mode := s.roomModes[roomID]
	start := s.battleStart[roomID]
	s.mu.Unlock()

	if mode != domain.ModeRanked {
		return
	}

	req := ApplyResultRequest{
		WinnerID:   winnerID,
		LoserID:    loserID,
		Mode:       mode,
		DurationMs: s.now().Sub(start).Milliseconds(),
	}

	go func() { //nolint:contextcheck,gosec // G118: fire-and-forget; timer callbacks carry no request context
		rCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := s.ratingsClient.ApplyResult(rCtx, req)
		if err != nil {
			s.logger.Error("ratings ApplyResult failed",
				zap.String("room_id", roomID),
				zap.Int64("winner_id", winnerID),
				zap.Int64("loser_id", loserID),
				zap.Error(err),
			)
		}
	}()
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
