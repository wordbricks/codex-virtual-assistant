package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func streamSSE(r io.Reader, fn func(assistant.RunEvent) bool) error {
	scanner := bufio.NewScanner(r)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, ":") {
			continue // keep-alive comment
		}

		if strings.HasPrefix(line, "event:") {
			continue // we only care about data lines
		}

		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
			continue
		}

		// empty line = end of event
		if line == "" && len(dataLines) > 0 {
			raw := strings.Join(dataLines, "\n")
			dataLines = dataLines[:0]

			var ev assistant.RunEvent
			if err := json.Unmarshal([]byte(raw), &ev); err != nil {
				continue // skip malformed events
			}
			if !fn(ev) {
				return nil
			}
		}
	}
	return scanner.Err()
}
