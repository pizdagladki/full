// Package domain holds the signaling service domain models, DTOs, and enums.
// No I/O — only in-memory types and logic.
package domain

import (
	"encoding/json"
	"errors"
)

// Sentinel domain errors.
var (
	// ErrInvalidRoomID is returned when a room_id is empty or exceeds maxRoomIDLen.
	ErrInvalidRoomID = errors.New("room_id must be non-empty and at most 128 characters")
	// ErrRoomFull is returned when a third peer tries to join an already-full room.
	ErrRoomFull = errors.New("room is full")
	// ErrNotMember is returned when a sender tries to relay to a room they have not joined.
	ErrNotMember = errors.New("not a member of this room")
	// ErrAlreadyInRoom is returned when a peer tries to join a different room while
	// already in one. A signaling connection belongs to exactly one room for its lifetime.
	ErrAlreadyInRoom = errors.New("already in a room; join a different room is not allowed")
	// ErrMatchFinished is returned when a blink/face_lost is reported after an outcome
	// has already been decided for the room (idempotent guard).
	ErrMatchFinished = errors.New("match outcome already decided")
)

const maxRoomIDLen = 128

// Message type constants for inbound messages.
const (
	TypeJoin = "join"
	TypeSDP  = "sdp"
	TypeICE  = "ice"
	// TypeBlink is sent by a client when the user blinks during a battle.
	TypeBlink = "blink"
	// TypeFaceLost is sent by a client when face tracking is lost during a battle.
	TypeFaceLost = "face_lost"
	// TypeOutcome is sent by the server to announce the battle result.
	TypeOutcome = "outcome"
)

// InboundEnvelope is parsed only for routing: it reads the type and room_id
// fields to decide how to dispatch the message. SDP and ICE payloads are
// forwarded VERBATIM (raw bytes) so the full original message is never
// re-marshaled.
type InboundEnvelope struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
}

// peerLeftMsg is the server-to-client notification when the peer disconnects.
type peerLeftMsg struct {
	Type string `json:"type"`
}

// outcomeMsg is the server-to-client authoritative battle result.
type outcomeMsg struct {
	Type     string `json:"type"`
	WinnerID int64  `json:"winner_id"`
	LoserID  int64  `json:"loser_id"`
}

// errMsg is the server-to-client error notification.
type errMsg struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// OutcomeBytes returns the marshaled {"type":"outcome","winner_id":...,"loser_id":...} frame bytes.
// Panics on marshal failure (struct has only primitive fields — cannot fail).
func OutcomeBytes(winnerID, loserID int64) []byte {
	b, err := json.Marshal(outcomeMsg{Type: TypeOutcome, WinnerID: winnerID, LoserID: loserID})
	if err != nil {
		panic("domain: marshal outcome: " + err.Error())
	}

	return b
}

// PeerLeftBytes returns the marshaled {"type":"peer_left"} frame bytes.
// Panics on marshal failure (struct has no dynamic fields — cannot fail).
func PeerLeftBytes() []byte {
	b, err := json.Marshal(peerLeftMsg{Type: "peer_left"})
	if err != nil {
		panic("domain: marshal peer_left: " + err.Error())
	}

	return b
}

// ErrorBytes returns the marshaled {"type":"error","error":<reason>} frame bytes.
func ErrorBytes(reason string) []byte {
	b, err := json.Marshal(errMsg{Type: "error", Error: reason})
	if err != nil {
		// Fall back to a static error — should never happen.
		return []byte(`{"type":"error","error":"internal error"}`)
	}

	return b
}

// ValidateRoomID returns ErrInvalidRoomID when roomID is empty or longer than
// maxRoomIDLen characters. The value flows into a Redis key (room:<roomID>:members)
// so bounded length is enforced here.
func ValidateRoomID(roomID string) error {
	if roomID == "" || len(roomID) > maxRoomIDLen {
		return ErrInvalidRoomID
	}

	return nil
}
