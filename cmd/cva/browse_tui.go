package main

type browseOutputMode string

const (
	browseOutputModeJSON browseOutputMode = "json"
	browseOutputModeTUI  browseOutputMode = "tui"
	browseOutputModeText browseOutputMode = "text"
)

func selectBrowseOutputMode(jsonMode bool, stdinIsTTY, stdoutIsTTY bool) browseOutputMode {
	if jsonMode {
		return browseOutputModeJSON
	}
	if stdinIsTTY && stdoutIsTTY {
		return browseOutputModeTUI
	}
	return browseOutputModeText
}
