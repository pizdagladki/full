package service

import "testing"

func TestHealthService_Check(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns ok", want: "ok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewHealthService().Check()
			if got.Status != tt.want {
				t.Errorf("Check().Status = %q, want %q", got.Status, tt.want)
			}
		})
	}
}
