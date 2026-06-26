package service

import (
	"context"
	"fmt"
	"os/exec"
)

type execFFmpegRunner struct {
	bin string
}

// NewExecFFmpegRunner returns an FFmpegRunner that invokes the ffmpeg binary at
// bin. Use exec.LookPath("ffmpeg") to resolve the path at startup.
func NewExecFFmpegRunner(bin string) FFmpegRunner {
	return &execFFmpegRunner{bin: bin}
}

// buildFFmpegArgs returns the slice of arguments for the ffmpeg conversion
// command. It is exported for testing the command-builder without invoking a
// real binary. The -loglevel error -hide_banner flags limit stderr volume to
// actual errors only, bounding the output captured by CombinedOutput.
func buildFFmpegArgs(inputPath, outputPath string) []string {
	return []string{
		"-y",
		"-loglevel", "error",
		"-hide_banner",
		"-i", inputPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outputPath,
	}
}

func (r *execFFmpegRunner) Convert(ctx context.Context, inputPath, outputPath string) error {
	args := buildFFmpegArgs(inputPath, outputPath)
	cmd := exec.CommandContext(ctx, r.bin, args...) //nolint:gosec // bin is resolved via LookPath at startup
	out, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, out)
	}

	return nil
}
