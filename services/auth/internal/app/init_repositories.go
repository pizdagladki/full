package app

import "github.com/pizdagladki/full/services/auth/internal/api/repository"

func (a *App) initRepositories() {
	a.userRepo = repository.NewUserRepository(a.pgxPool)
}
