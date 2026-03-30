package wtl

import (
	"fmt"
	"strconv"
	"strings"
)

type chromeTabGroupSnapshot struct {
	selected    *chromeTabGroupNode
	groupHeader *chromeTabGroupNode
	groupTab    *chromeTabGroupNode
}

type chromeTabGroupNode struct {
	kind        string
	description string
	x           int
	y           int
	width       int
	height      int
	inGroup     bool
}

func (n *chromeTabGroupNode) center() (int, int) {
	if n == nil {
		return 0, 0
	}
	return n.x + (n.width / 2), n.y + (n.height / 2)
}

func (s chromeTabGroupSnapshot) target() *chromeTabGroupNode {
	if s.groupTab != nil {
		return s.groupTab
	}
	return s.groupHeader
}

func parseChromeTabGroupSnapshot(raw string) (chromeTabGroupSnapshot, error) {
	var snapshot chromeTabGroupSnapshot
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 7 {
			return chromeTabGroupSnapshot{}, fmt.Errorf("unexpected chrome tab group line %q", line)
		}

		x, err := strconv.Atoi(parts[2])
		if err != nil {
			return chromeTabGroupSnapshot{}, fmt.Errorf("parse x for %q: %w", line, err)
		}
		y, err := strconv.Atoi(parts[3])
		if err != nil {
			return chromeTabGroupSnapshot{}, fmt.Errorf("parse y for %q: %w", line, err)
		}
		width, err := strconv.Atoi(parts[4])
		if err != nil {
			return chromeTabGroupSnapshot{}, fmt.Errorf("parse width for %q: %w", line, err)
		}
		height, err := strconv.Atoi(parts[5])
		if err != nil {
			return chromeTabGroupSnapshot{}, fmt.Errorf("parse height for %q: %w", line, err)
		}
		inGroup, err := strconv.ParseBool(parts[6])
		if err != nil {
			return chromeTabGroupSnapshot{}, fmt.Errorf("parse in-group flag for %q: %w", line, err)
		}

		node := &chromeTabGroupNode{
			kind:        parts[0],
			description: parts[1],
			x:           x,
			y:           y,
			width:       width,
			height:      height,
			inGroup:     inGroup,
		}
		switch node.kind {
		case "selected":
			snapshot.selected = node
		case "group-header":
			snapshot.groupHeader = node
		case "group-tab":
			if snapshot.groupTab == nil {
				snapshot.groupTab = node
			}
		default:
			return chromeTabGroupSnapshot{}, fmt.Errorf("unexpected chrome tab group node kind %q", node.kind)
		}
	}
	return snapshot, nil
}

func shouldAttemptChromeTabGrouping(groupName, command string) bool {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return false
	}
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "agent-browser") {
		return false
	}
	for _, needle := range []string{
		"agent-browser open",
		"agent-browser goto",
		"agent-browser navigate",
		"agent-browser click",
		"agent-browser press",
		"agent-browser keyboard",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
