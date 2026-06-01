package tunnel

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EnsureBinary checks if cloudflared is available in the system PATH.
// If not found, it checks ~/.1agents/bin/cloudflared, and downloads
// the correct binary for the host's OS and architecture if missing.
func EnsureBinary() (string, error) {
	// 1. Check if already available in the system PATH
	if path, err := exec.LookPath("cloudflared"); err == nil {
		log.Printf("[tunnel] Found system cloudflared in PATH: %s", path)
		return path, nil
	}

	// 2. Resolve default user directory: ~/.1agents/bin/
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	binDir := filepath.Join(home, ".1agents", "bin")
	binaryName := "cloudflared"
	if runtime.GOOS == "windows" {
		binaryName = "cloudflared.exe"
	}
	finalPath := filepath.Join(binDir, binaryName)

	// 3. Check if already downloaded
	if _, err := os.Stat(finalPath); err == nil {
		return finalPath, nil
	}

	// 4. Download based on OS and architecture
	log.Printf("[tunnel] cloudflared not found in PATH or local bin. Starting automatic download...")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %w", err)
	}

	var downloadURL string
	switch runtime.GOOS {
	case "darwin":
		// Cloudflare produces darwin-amd64 which runs seamlessly on both Intel and Apple Silicon (via Rosetta)
		downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-amd64"
	case "linux":
		if runtime.GOARCH == "arm64" {
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64"
		} else {
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
		}
	case "windows":
		downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"
	default:
		return "", fmt.Errorf("unsupported operating system for automatic cloudflared download: %s", runtime.GOOS)
	}

	log.Printf("[tunnel] Downloading cloudflared binary from: %s", downloadURL)
	
	// Stream to a temp file first to prevent corruption from partial downloads
	tempFile, err := os.CreateTemp(binDir, "cloudflared-tmp-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file for download: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		tempFile.Close()
		_ = os.Remove(tempPath) // Clean up temp file if not renamed
	}()

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download cloudflared binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download: server returned status code %d", resp.StatusCode)
	}

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		return "", fmt.Errorf("failed to write cloudflared binary to disk: %w", err)
	}
	tempFile.Close()

	// 5. Rename to final executable path and make executable
	if err := os.Rename(tempPath, finalPath); err != nil {
		return "", fmt.Errorf("failed to move cloudflared binary to destination: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(finalPath, 0755); err != nil {
			return "", fmt.Errorf("failed to set executable permission on cloudflared: %w", err)
		}
	}

	log.Printf("[tunnel] Successfully downloaded and installed cloudflared to: %s", finalPath)
	return finalPath, nil
}
