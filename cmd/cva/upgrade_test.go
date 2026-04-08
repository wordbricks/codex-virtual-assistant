package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReleaseAssetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{name: "darwin amd64", goos: "darwin", goarch: "amd64", want: "cva-darwin-x64"},
		{name: "darwin arm64", goos: "darwin", goarch: "arm64", want: "cva-darwin-arm64"},
		{name: "linux amd64", goos: "linux", goarch: "amd64", want: "cva-linux-x64"},
		{name: "linux arm64", goos: "linux", goarch: "arm64", want: "cva-linux-arm64"},
		{name: "windows amd64", goos: "windows", goarch: "amd64", want: "cva-win32-x64.exe"},
		{name: "windows arm64", goos: "windows", goarch: "arm64", want: "cva-win32-arm64.exe"},
		{name: "unsupported platform", goos: "freebsd", goarch: "amd64", wantErr: true},
		{name: "unsupported arch", goos: "darwin", goarch: "386", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := releaseAssetName(tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("releaseAssetName(%q, %q) succeeded unexpectedly", tt.goos, tt.goarch)
				}
				return
			}
			if err != nil {
				t.Fatalf("releaseAssetName(%q, %q) error = %v", tt.goos, tt.goarch, err)
			}
			if got != tt.want {
				t.Fatalf("releaseAssetName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestUpgraderUpgradeReplacesExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self replacement semantics differ on Windows")
	}

	originalVersion := version
	version = "0.1.0"
	t.Cleanup(func() {
		version = originalVersion
	})

	assetName, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("releaseAssetName() error = %v", err)
	}

	tempDir := t.TempDir()
	executable := filepath.Join(tempDir, "cva")
	if err := os.WriteFile(executable, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	downloadHits := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","assets":[{"name":"` + assetName + `","browser_download_url":"` + server.URL + `/asset"}]}`))
		case "/asset":
			downloadHits++
			_, _ = w.Write([]byte("new-binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	upgrader := &upgrader{
		client:           server.Client(),
		latestReleaseURL: server.URL + "/latest",
		executablePath: func() (string, error) {
			return executable, nil
		},
	}

	result, err := upgrader.upgrade(context.Background())
	if err != nil {
		t.Fatalf("upgrade() error = %v", err)
	}
	if !result.Upgraded {
		t.Fatalf("upgrade() Upgraded = false, want true")
	}
	if result.LatestVersion != "0.2.0" {
		t.Fatalf("upgrade() LatestVersion = %q, want %q", result.LatestVersion, "0.2.0")
	}
	if got := string(mustReadFile(t, executable)); got != "new-binary" {
		t.Fatalf("upgraded executable contents = %q, want %q", got, "new-binary")
	}
	if downloadHits != 1 {
		t.Fatalf("download hits = %d, want 1", downloadHits)
	}
}

func TestUpgraderUpgradeSkipsCurrentVersion(t *testing.T) {
	originalVersion := version
	version = "0.2.0"
	t.Cleanup(func() {
		version = originalVersion
	})

	assetName, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("releaseAssetName() error = %v", err)
	}

	tempDir := t.TempDir()
	executable := filepath.Join(tempDir, "cva")
	if err := os.WriteFile(executable, []byte("existing-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	downloadHits := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","assets":[{"name":"` + assetName + `","browser_download_url":"` + server.URL + `/asset"}]}`))
		case "/asset":
			downloadHits++
			_, _ = w.Write([]byte("new-binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	upgrader := &upgrader{
		client:           server.Client(),
		latestReleaseURL: server.URL + "/latest",
		executablePath: func() (string, error) {
			return executable, nil
		},
	}

	result, err := upgrader.upgrade(context.Background())
	if err != nil {
		t.Fatalf("upgrade() error = %v", err)
	}
	if result.Upgraded {
		t.Fatalf("upgrade() Upgraded = true, want false")
	}
	if got := string(mustReadFile(t, executable)); got != "existing-binary" {
		t.Fatalf("executable contents = %q, want %q", got, "existing-binary")
	}
	if downloadHits != 0 {
		t.Fatalf("download hits = %d, want 0", downloadHits)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return data
}
