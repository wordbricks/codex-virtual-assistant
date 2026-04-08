package main

import "testing"

func TestCurrentVersionInfo(t *testing.T) {
	t.Parallel()

	originalVersion := version
	originalCommit := commit
	originalBuildDate := buildDate
	t.Cleanup(func() {
		version = originalVersion
		commit = originalCommit
		buildDate = originalBuildDate
	})

	version = "1.2.3"
	commit = "abc1234"
	buildDate = "2026-04-08T00:00:00Z"

	info := currentVersionInfo()
	if info.Version != "1.2.3" || info.Commit != "abc1234" || info.BuildDate != "2026-04-08T00:00:00Z" {
		t.Fatalf("currentVersionInfo() = %#v", info)
	}
}

func TestFormatVersionText(t *testing.T) {
	t.Parallel()

	text := formatVersionText(versionInfo{
		Version:   "1.2.3",
		Commit:    "abc1234",
		BuildDate: "2026-04-08T00:00:00Z",
	})
	if text == "" {
		t.Fatal("formatVersionText() returned empty string")
	}
}
