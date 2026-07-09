package speechclip

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ffmpegExe resolves the ffmpeg binary bundled with FunClip's venv (imageio-ffmpeg),
// so recording mux/transcode reuses the same toolchain as transcription.
func ffmpegExe() (string, error) {
	fdir, err := funclipDir()
	if err != nil {
		return "", err
	}
	py := filepath.Join(fdir, ".venv", "bin", "python")
	out, err := exec.Command(py, "-c", "import imageio_ffmpeg; print(imageio_ffmpeg.get_ffmpeg_exe())").Output()
	if err != nil {
		return "", fmt.Errorf("speech_clip: resolve ffmpeg: %v", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", fmt.Errorf("speech_clip: empty ffmpeg path")
	}
	return p, nil
}

// muxAV combines a (silent) screen video with a separately-recorded audio track
// into one webm at outPath — the playable 口播 clip used for 混剪, whose audio is
// also what transcription extracts. Streams are copied, not re-encoded.
func muxAV(videoPath, audioPath, outPath string) error {
	ff, err := ffmpegExe()
	if err != nil {
		return err
	}
	cmd := exec.Command(ff,
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-shortest",
		outPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("speech_clip: ffmpeg mux: %v: %s", err, tail(string(out), 500))
	}
	return nil
}
