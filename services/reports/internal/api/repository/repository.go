// Package repository holds the reports service data access (hand-written SQL
// via pgx, mapping rows to domain models). Repository interfaces are added here
// by downstream resource slices via the new-resource skill.
package repository

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks
