package app

import "github.com/pizdagladki/full/services/koth/internal/api/repository"

func (a *App) initRepositories() {
	a.rankRepo = repository.NewRankRepository(a.pgxPool)
	a.hillRepo = repository.NewHillRepository(a.pgxPool)
	a.sessionRepo = repository.NewSessionRepository(a.redisClient)
}
