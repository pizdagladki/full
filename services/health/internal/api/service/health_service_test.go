package service

import "testing"

func TestHealthService_Check(t *testing.T) {
	got := NewHealthService().Check()
	if got.Status != "ok" {
		t.Errorf("status = %q, want %q", got.Status, "ok")
	}
}
