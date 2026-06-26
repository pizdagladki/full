package domain

import "testing"

func TestBuildObjectKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		userID int64
		id     string
		want   string
	}{
		{
			name:   "standard key",
			userID: 42,
			id:     "abc-123",
			want:   "clips/42/abc-123.webm",
		},
		{
			name:   "zero user id",
			userID: 0,
			id:     "xyz",
			want:   "clips/0/xyz.webm",
		},
		{
			name:   "large user id",
			userID: 9999999,
			id:     "uuid-goes-here",
			want:   "clips/9999999/uuid-goes-here.webm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildObjectKey(tt.userID, tt.id)
			if got != tt.want {
				t.Errorf("BuildObjectKey(%d, %q) = %q, want %q", tt.userID, tt.id, got, tt.want)
			}
		})
	}
}

func TestValidContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ct   string
		want bool
	}{
		{
			name: "exact webm",
			ct:   "video/webm",
			want: true,
		},
		{
			name: "webm with codecs suffix",
			ct:   "video/webm; codecs=vp8,opus",
			want: true,
		},
		{
			name: "webm with spaces around semicolon",
			ct:   "video/webm ; codecs=vp9",
			want: true,
		},
		{
			name: "mp4 is rejected",
			ct:   "video/mp4",
			want: false,
		},
		{
			name: "empty string is rejected",
			ct:   "",
			want: false,
		},
		{
			name: "application/json is rejected",
			ct:   "application/json",
			want: false,
		},
		{
			name: "partial match is rejected",
			ct:   "video/webm-extra",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ValidContentType(tt.ct)
			if got != tt.want {
				t.Errorf("ValidContentType(%q) = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
}
