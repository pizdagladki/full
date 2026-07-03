package service

import "time"

// ExportedBuildFFmpegArgs exposes buildFFmpegArgs for white-box testing of the
// command-builder without invoking a real ffmpeg binary.
func ExportedBuildFFmpegArgs(inputPath, outputPath string) []string {
	return buildFFmpegArgs(inputPath, outputPath)
}

// SetKingClipServiceClock overrides the internal clock of a KingClipService
// built by NewKingClipService, so tests can assert deterministic ExpiresAt
// values. It panics if svc was not built by NewKingClipService.
func SetKingClipServiceClock(svc KingClipService, now func() time.Time) {
	svc.(*kingClipService).now = now //nolint:forcetypeassert // test-only helper, panics on misuse by design
}
