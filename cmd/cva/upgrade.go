package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const latestReleaseURL = "https://api.github.com/repos/wordbricks/codex-virtual-assistant/releases/latest"

type upgradeResult struct {
	CurrentVersion         string `json:"current_version"`
	LatestVersion          string `json:"latest_version"`
	Executable             string `json:"executable"`
	AssetName              string `json:"asset_name"`
	AgentBrowserExecutable string `json:"agent_browser_executable,omitempty"`
	AgentBrowserAssetName  string `json:"agent_browser_asset_name,omitempty"`
	Upgraded               bool   `json:"upgraded"`
	CVAUpgraded            bool   `json:"cva_upgraded"`
	AgentBrowserUpgraded   bool   `json:"agent_browser_upgraded"`
	Message                string `json:"message"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type upgrader struct {
	client           *http.Client
	latestReleaseURL string
	executablePath   func() (string, error)
}

func newUpgrader() *upgrader {
	return &upgrader{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		latestReleaseURL: latestReleaseURL,
		executablePath:   os.Executable,
	}
}

func cmdUpgrade(ctx context.Context, args []string, jsonMode bool) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: cva upgrade")
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("cva upgrade is not supported on Windows yet; reinstall the latest release instead")
	}

	result, err := newUpgrader().upgrade(ctx)
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(result)
	}
	fmt.Print(formatUpgradeText(result))
	return nil
}

func (u *upgrader) upgrade(ctx context.Context) (upgradeResult, error) {
	result := upgradeResult{
		CurrentVersion: version,
	}

	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return result, err
	}
	result.LatestVersion = normalizeVersion(release.TagName)

	assetName, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return result, err
	}
	result.AssetName = assetName
	agentBrowserAssetName, err := releaseAgentBrowserAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return result, err
	}
	result.AgentBrowserAssetName = agentBrowserAssetName

	executable, err := u.executablePath()
	if err != nil {
		return result, fmt.Errorf("resolve executable path: %w", err)
	}
	result.Executable = executable
	result.AgentBrowserExecutable = agentBrowserUpgradePath(executable, runtime.GOOS)

	currentVersionIsLatest := normalizeVersion(version) == result.LatestVersion
	agentBrowserExists := false
	if strings.TrimSpace(result.AgentBrowserExecutable) != "" {
		if info, err := os.Stat(result.AgentBrowserExecutable); err == nil && !info.IsDir() {
			agentBrowserExists = true
		}
	}
	if currentVersionIsLatest && agentBrowserExists {
		result.Message = fmt.Sprintf("cva %s is already up to date", result.LatestVersion)
		return result, nil
	}

	asset, err := release.assetByName(assetName)
	if err != nil {
		return result, err
	}
	agentBrowserAsset, err := release.assetByName(agentBrowserAssetName)
	if err != nil {
		return result, err
	}

	var cvaTempPath string
	if !currentVersionIsLatest {
		cvaTempPath, err = u.downloadAsset(ctx, asset.BrowserDownloadURL, executable)
		if err != nil {
			return result, err
		}
		defer os.Remove(cvaTempPath)
	}

	agentBrowserTempPath, err := u.downloadAsset(ctx, agentBrowserAsset.BrowserDownloadURL, result.AgentBrowserExecutable)
	if err != nil {
		return result, err
	}
	defer os.Remove(agentBrowserTempPath)

	if err := os.Rename(agentBrowserTempPath, result.AgentBrowserExecutable); err != nil {
		return result, fmt.Errorf("replace agent-browser executable: %w", err)
	}
	result.AgentBrowserUpgraded = true

	if cvaTempPath != "" {
		if err := os.Rename(cvaTempPath, executable); err != nil {
			return result, fmt.Errorf("replace executable: %w", err)
		}
		result.CVAUpgraded = true
	}

	result.Upgraded = result.CVAUpgraded || result.AgentBrowserUpgraded
	switch {
	case result.CVAUpgraded && result.AgentBrowserUpgraded:
		result.Message = fmt.Sprintf("upgraded cva and agent-browser from %s to %s", displayVersion(version), result.LatestVersion)
	case result.CVAUpgraded:
		result.Message = fmt.Sprintf("upgraded cva from %s to %s", displayVersion(version), result.LatestVersion)
	case result.AgentBrowserUpgraded:
		result.Message = fmt.Sprintf("installed latest agent-browser for cva %s", result.LatestVersion)
	default:
		result.Message = fmt.Sprintf("cva %s is already up to date", result.LatestVersion)
	}
	return result, nil
}

func (u *upgrader) fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.latestReleaseURL, nil)
	if err != nil {
		return githubRelease{}, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fmt.Sprintf("cva/%s", displayVersion(version)))
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := u.client.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return githubRelease{}, fmt.Errorf("fetch latest release: unexpected HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("decode latest release: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return githubRelease{}, fmt.Errorf("latest release response did not include a tag name")
	}
	return release, nil
}

func (u *upgrader) downloadAsset(ctx context.Context, url string, executable string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build asset download request: %w", err)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("cva/%s", displayVersion(version)))

	resp, err := u.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download latest binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("download latest binary: unexpected HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tempFile, err := os.CreateTemp(filepath.Dir(executable), "cva-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("create temp executable: %w", err)
	}
	tempPath := tempFile.Name()

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		tempFile.Close()
		return "", fmt.Errorf("write temp executable: %w", err)
	}
	if err := tempFile.Chmod(0o755); err != nil {
		tempFile.Close()
		return "", fmt.Errorf("chmod temp executable: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("close temp executable: %w", err)
	}

	return tempPath, nil
}

func (r githubRelease) assetByName(name string) (githubAsset, error) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	return githubAsset{}, fmt.Errorf("latest release %s does not contain asset %s", r.TagName, name)
}

func releaseAssetName(goos string, goarch string) (string, error) {
	return releaseBinaryAssetName("cva", goos, goarch)
}

func releaseAgentBrowserAssetName(goos string, goarch string) (string, error) {
	return releaseBinaryAssetName("agent-browser", goos, goarch)
}

func releaseBinaryAssetName(prefix string, goos string, goarch string) (string, error) {
	platform := goos
	switch goos {
	case "darwin", "linux":
	case "windows":
		platform = "win32"
	default:
		return "", fmt.Errorf("unsupported platform for upgrade: %s/%s", goos, goarch)
	}

	arch := goarch
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
	default:
		return "", fmt.Errorf("unsupported platform for upgrade: %s/%s", goos, goarch)
	}

	suffix := ""
	if platform == "win32" {
		suffix = ".exe"
	}
	return fmt.Sprintf("%s-%s-%s%s", prefix, platform, arch, suffix), nil
}

func agentBrowserExecutableName(goos string) string {
	if goos == "windows" {
		return "agent-browser.exe"
	}
	return "agent-browser"
}

func agentBrowserUpgradePath(cvaExecutable string, goos string) string {
	return filepath.Join(filepath.Dir(cvaExecutable), agentBrowserExecutableName(goos))
}

func normalizeVersion(raw string) string {
	return strings.TrimPrefix(strings.TrimSpace(raw), "v")
}

func displayVersion(raw string) string {
	value := normalizeVersion(raw)
	if value == "" {
		return "unknown"
	}
	return value
}

func formatUpgradeText(result upgradeResult) string {
	if result.Upgraded {
		lines := []string{
			result.Message,
			fmt.Sprintf("executable: %s", result.Executable),
			fmt.Sprintf("asset: %s", result.AssetName),
		}
		if strings.TrimSpace(result.AgentBrowserExecutable) != "" {
			lines = append(lines,
				fmt.Sprintf("agent-browser executable: %s", result.AgentBrowserExecutable),
				fmt.Sprintf("agent-browser asset: %s", result.AgentBrowserAssetName),
			)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	return fmt.Sprintf("%s\n", result.Message)
}
