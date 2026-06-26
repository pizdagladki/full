package service

// ExportedBuildFFmpegArgs exposes buildFFmpegArgs for white-box testing of the
// command-builder without invoking a real ffmpeg binary.
func ExportedBuildFFmpegArgs(inputPath, outputPath string) []string {
	return buildFFmpegArgs(inputPath, outputPath)
}
