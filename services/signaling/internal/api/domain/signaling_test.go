package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateRoomID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		roomID  string
		wantErr error
	}{
		{
			name:    "valid short room id", // criterion: 1
			roomID:  "room-abc",
			wantErr: nil,
		},
		{
			name:    "valid max length room id", // criterion: 1
			roomID:  strings.Repeat("a", 128),
			wantErr: nil,
		},
		{
			name:    "empty room id returns ErrInvalidRoomID", // criterion: 1 — fails if validation not enforced
			roomID:  "",
			wantErr: ErrInvalidRoomID,
		},
		{
			name:    "too long room id returns ErrInvalidRoomID", // criterion: 1 — fails if length not bounded
			roomID:  strings.Repeat("x", 129),
			wantErr: ErrInvalidRoomID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateRoomID(tt.roomID)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ValidateRoomID(%q) error = nil, want %v", tt.roomID, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ValidateRoomID(%q) unexpected error = %v", tt.roomID, err)
			}
		})
	}
}

func TestPeerLeftBytes(t *testing.T) {
	t.Parallel()

	b := PeerLeftBytes()
	if b == nil {
		t.Fatal("PeerLeftBytes() = nil, want non-nil")
	}

	var msg map[string]string
	if err := json.Unmarshal(b, &msg); err != nil {
		t.Fatalf("PeerLeftBytes() not valid JSON: %v", err)
	}

	if msg["type"] != "peer_left" {
		t.Errorf("type = %q, want %q", msg["type"], "peer_left")
	}
}

func TestErrorBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reason  string
		wantMsg string
	}{
		{
			name:    "error bytes contains reason",
			reason:  "room is full",
			wantMsg: "room is full",
		},
		{
			name:    "error bytes has type=error",
			reason:  "not a member",
			wantMsg: "not a member",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ErrorBytes(tt.reason)
			if b == nil {
				t.Fatal("ErrorBytes() = nil")
			}

			var msg map[string]string
			if err := json.Unmarshal(b, &msg); err != nil {
				t.Fatalf("ErrorBytes() not valid JSON: %v", err)
			}

			if msg["type"] != "error" {
				t.Errorf("type = %q, want %q", msg["type"], "error")
			}

			if msg["error"] != tt.wantMsg {
				t.Errorf("error = %q, want %q", msg["error"], tt.wantMsg)
			}
		})
	}
}
