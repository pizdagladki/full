// Package service holds the reports service business logic (orchestrating
// repositories and external integrations). Service interfaces are added here by
// downstream resource slices via the new-resource skill.
package service

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks
