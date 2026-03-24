package updater

import (
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
	defer resp.Body.Close()

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

// SelfUpdate downloads the latest binary and replaces the current one.
func SelfUpdate(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("no download URL for this platform")
	}

	// Download to a temp file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	// Get current binary path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	// Write to temp file next to the binary
	tmpPath := exePath + ".update"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writing update: %w", err)
	}

	// Replace the current binary: rename old to .bak, rename new to current
	bakPath := exePath + ".bak"
	os.Remove(bakPath) // clean up any previous backup

	if err := os.Rename(exePath, bakPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("backing up current binary: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		// Try to restore backup
		os.Rename(bakPath, exePath)
		return fmt.Errorf("installing update: %w", err)
	}

	// Clean up backup
	os.Remove(bakPath)

	return nil
}

func platformSuffix() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	return goos + "-" + goarch
}
