package storage

import (
	"context"
	"testing"
	"time"
)

func TestNew_UnreachableEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "unreachable endpoint returns error and nil client",
			cfg: Config{
				Endpoint:  "127.0.0.1:1",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Bucket:    "test-bucket",
				UseSSL:    false,
			},
		},
		{
			name: "unreachable ssl endpoint returns error and nil client",
			cfg: Config{
				Endpoint:  "127.0.0.1:1",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Bucket:    "other-bucket",
				UseSSL:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			client, err := New(ctx, tt.cfg)
			if err == nil {
				t.Fatal("New() error = nil, want error against an unreachable MinIO endpoint")
			}
			if client != nil {
				t.Error("New() client is non-nil after a failed connect, want nil")
			}
		})
	}
}
