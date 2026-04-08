package main

import "fmt"

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

func currentVersionInfo() versionInfo {
	return versionInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}
}

func formatVersionText(info versionInfo) string {
	return fmt.Sprintf("cva %s\ncommit: %s\nbuilt: %s\n", info.Version, info.Commit, info.BuildDate)
}
