// Package repository holds the media service data access (hand-written SQL via
// pgx, mapping rows to domain models, and MinIO object operations for win-clip
// upload/download). Repository interfaces are added here by downstream resource
// slices via the new-resource skill.
package repository
