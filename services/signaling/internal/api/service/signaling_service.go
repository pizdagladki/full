package service

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/signaling/internal/api/domain"
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
)

// signalingService implements SignalingService.
type signalingService struct {
	logger   *zap.Logger
	roomRepo repository.RoomRepository

	// rooms is the in-process registry: roomID → (userID → Conn).
	// Single-instance only; K8s scale-out would need a shared pub/sub store.
	mu    sync.Mutex
	rooms map[string]map[int64]Conn
}

// NewSignalingService constructs a SignalingService.
func NewSignalingService(
	logger *zap.Logger,
	roomRepo repository.RoomRepository,
) SignalingService {
	return &signalingService{
		logger:   logger,
		roomRepo: roomRepo,
		rooms:    make(map[string]map[int64]Conn),
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

// Leave is called on disconnect: notifies the peer with peer_left, removes the
// sender from the in-process hub, closes the peer, and deletes the room from Redis.
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

	// Remove the in-process room entry entirely.
	delete(s.rooms, roomID)
	s.mu.Unlock()

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
