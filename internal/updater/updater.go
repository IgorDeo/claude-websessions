package updater

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

const (
	repoOwner = "IgorDeo"
	repoName  = "claude-websessions"
	apiURL    = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
)

type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	UpdateAvail    bool
	ReleaseURL     string
	ReleaseNotes   string
	DownloadURL    string
	AssetName      string
	AssetSize      int64
}

// CheckForUpdate queries GitHub for the latest release and compares with current version.
func CheckForUpdate(currentVersion string) (*UpdateInfo, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "websessions/"+currentVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing release: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		UpdateAvail:    latest != current && currentVersion != "dev",
		ReleaseURL:     release.HTMLURL,
		ReleaseNotes:   release.Body,
	}

	// Find the right asset for this platform
	suffix := platformSuffix()
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, suffix) && !strings.Contains(asset.Name, "checksums") {
			info.DownloadURL = asset.BrowserDownloadURL
			info.AssetName = asset.Name
			info.AssetSize = asset.Size
			break
		}
	}

	return info, nil
}

// verifyChecksum verifies a file's SHA-256 hash against the expected hex string.
// If expected is empty, verification is skipped and nil is returned.
func verifyChecksum(filePath, expectedSHA256Hex string) error {
	if expectedSHA256Hex == "" {
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file for checksum: %w", err)
	}
	defer f.Close() //nolint:errcheck

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing file: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != strings.ToLower(expectedSHA256Hex) {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", actual, expectedSHA256Hex)
	}
	return nil
}

// safeSelfUpdate atomically replaces exePath with newData, verifying checksum if provided.
// On failure after the backup rename, it attempts to restore from .bak.
// The .bak file is kept for manual rollback.
func safeSelfUpdate(exePath string, newData []byte, expectedHash string) error {
	if len(newData) == 0 {
		return fmt.Errorf("new binary data is empty")
	}

	tmpPath := exePath + ".update"
	bakPath := exePath + ".bak"

	// Write new binary to temp file
	if err := os.WriteFile(tmpPath, newData, 0755); err != nil {
		return fmt.Errorf("writing update file: %w", err)
	}

	// Verify checksum before replacing
	if err := verifyChecksum(tmpPath, expectedHash); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Rename current binary to .bak
	if err := os.Rename(exePath, bakPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Rename .update to current binary path
	if err := os.Rename(tmpPath, exePath); err != nil {
		// Attempt rollback
		_ = os.Rename(bakPath, exePath)
		return fmt.Errorf("installing update: %w", err)
	}

	// Keep .bak for manual rollback — do NOT delete it

	return nil
}

// fetchExpectedHash downloads the checksums.txt from the same release and returns
// the SHA-256 hash for the asset matching the given downloadURL's filename.
// Returns "" on any failure (best-effort).
func fetchExpectedHash(downloadURL string) string {
	if downloadURL == "" {
		return ""
	}

	// Replace the asset filename in the URL with "checksums.txt"
	// Use strings.LastIndex to avoid mangling the "://" in the scheme.
	idx := strings.LastIndex(downloadURL, "/")
	if idx < 0 {
		return ""
	}
	checksumsURL := downloadURL[:idx] + "/checksums.txt"

	resp, err := http.Get(checksumsURL) //nolint:noctx
	if err != nil {
		return ""
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		return ""
	}

	// Determine the asset filename we're looking for
	assetName := downloadURL[idx+1:]

	// Parse lines in format "hash  filename"
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			return parts[0]
		}
	}

	return ""
}

// SelfUpdate downloads the latest binary and replaces the current one safely.
func SelfUpdate(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("no download URL for this platform")
	}

	// Try to fetch expected checksum (best-effort)
	expectedHash := fetchExpectedHash(downloadURL)

	// Download the new binary
	resp, err := http.Get(downloadURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	newData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading update: %w", err)
	}

	// Get current binary path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	return safeSelfUpdate(exePath, newData, expectedHash)
}

func platformSuffix() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	return goos + "-" + goarch
}
