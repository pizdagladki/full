// Package domain holds the media service domain models, DTOs, and enums. Entity
// types (win-clip upload metadata, playback records, conversion jobs) are added
// here by downstream resource slices via the new-resource skill.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ContentTypeWebM is the only accepted content type for clip uploads.
const ContentTypeWebM = "video/webm"

// MaxClipsPerUser is the maximum number of clips a user may have stored at once.
// Uploads beyond this limit trigger FIFO eviction of the oldest clips.
const MaxClipsPerUser = 10

// ErrInvalidContentType is returned when the uploaded file is not a WebM video.
var ErrInvalidContentType = errors.New("invalid content type: must be video/webm")

// ErrTooLarge is returned when the upload exceeds the configured size limit.
var ErrTooLarge = errors.New("upload too large")

// Clip represents a stored win-clip with its metadata.
type Clip struct {
	ID          int64
	UserID      int64
	ObjectKey   string
	Mode        string
	Result      string
	ContentType string
	SizeBytes   int64
	CreatedAt   time.Time
}

// BuildObjectKey returns the MinIO object key for a clip.
func BuildObjectKey(userID int64, id string) string {
	return fmt.Sprintf("clips/%d/%s.webm", userID, id)
}

// ValidContentType reports whether ct is an accepted content type for clips.
// It allows "video/webm" and tolerates a "; codecs=..." suffix by matching the
// media type before the first semicolon.
func ValidContentType(ct string) bool {
	before, _, _ := strings.Cut(ct, ";")

	return strings.TrimSpace(before) == ContentTypeWebM
}
