package app

import (
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
)

func (a *App) initRepositories() {
	a.sessionRepo = repository.NewSessionRepository(a.redisClient)
	a.roomRepo = repository.NewRoomRepository(a.redisClient, a.cfg.Signaling.RoomTTL)
}
