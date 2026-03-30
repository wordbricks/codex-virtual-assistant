//go:build darwin

package wtl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const chromeTabGroupSnapshotScript = `on run argv
	set targetGroupName to item 1 of argv
	set tabChar to character id 9
	tell application "System Events"
		if not (exists process "Google Chrome") then
			return ""
		end if
		tell process "Google Chrome"
			set frontmost to true
			delay 0.2
			if (count of windows) is 0 then
				return ""
			end if
			tell front window
				set outputLines to {}
				set theItems to entire contents
				repeat with i from 1 to count of theItems
					set e to item i of theItems
					try
						if subrole of e is "AXTabButton" then
							set d to description of e
							set p to position of e
							set sz to size of e
							set isSelected to selected of e as text
							set inGroup to "false"
							if d contains ("Part of group " & targetGroupName) then
								set inGroup to "true"
								set end of outputLines to "group-tab" & tabChar & d & tabChar & (item 1 of p as text) & tabChar & (item 2 of p as text) & tabChar & (item 1 of sz as text) & tabChar & (item 2 of sz as text) & tabChar & inGroup
							end if
							if isSelected is "true" then
								set end of outputLines to "selected" & tabChar & d & tabChar & (item 1 of p as text) & tabChar & (item 2 of p as text) & tabChar & (item 1 of sz as text) & tabChar & (item 2 of sz as text) & tabChar & inGroup
							end if
						end if
					end try
					try
						if role of e is "AXTabGroup" then
							set d to description of e
							if d contains targetGroupName then
								set p to position of e
								set sz to size of e
								set end of outputLines to "group-header" & tabChar & d & tabChar & (item 1 of p as text) & tabChar & (item 2 of p as text) & tabChar & (item 1 of sz as text) & tabChar & (item 2 of sz as text) & tabChar & "true"
							end if
						end if
					end try
				end repeat
				set AppleScript's text item delimiters to linefeed
				set joinedOutput to outputLines as text
				set AppleScript's text item delimiters to ""
				return joinedOutput
			end tell
		end tell
	end tell
end run
`

const chromeTabGroupDragScript = `import CoreGraphics
import Foundation

func post(_ type: CGEventType, x: CGFloat, y: CGFloat) {
    guard let event = CGEvent(mouseEventSource: nil, mouseType: type, mouseCursorPosition: CGPoint(x: x, y: y), mouseButton: .left) else {
        return
    }
    event.post(tap: .cghidEventTap)
}

let args = CommandLine.arguments
guard args.count == 5,
      let startX = Double(args[1]),
      let startY = Double(args[2]),
      let endX = Double(args[3]),
      let endY = Double(args[4]) else {
    throw NSError(domain: "chrome-tab-group", code: 64)
}

let start = CGPoint(x: startX, y: startY)
let end = CGPoint(x: endX, y: endY)

post(.mouseMoved, x: start.x, y: start.y)
usleep(100_000)
post(.leftMouseDown, x: start.x, y: start.y)
usleep(150_000)

for step in 1...18 {
    let progress = CGFloat(step) / 18.0
    let x = start.x + ((end.x - start.x) * progress)
    let y = start.y + ((end.y - start.y) * progress)
    post(.leftMouseDragged, x: x, y: y)
    usleep(25_000)
}

usleep(250_000)
post(.leftMouseUp, x: end.x, y: end.y)
`

const chromeTabGroupCreateScript = `on run argv
	set targetGroupName to item 1 of argv
	tell application "System Events"
		if not (exists process "Google Chrome") then
			return ""
		end if
		tell process "Google Chrome"
			set frontmost to true
			delay 0.2
			if (count of windows) is 0 then
				return ""
			end if
			perform action "AXPress" of menu bar item "Tab" of menu bar 1
			delay 0.2
			click menu item "Group Tab" of menu "Tab" of menu bar item "Tab" of menu bar 1
			delay 0.2
			keystroke targetGroupName
			key code 36
			delay 0.2
			return "created"
		end tell
	end tell
end run
`

func moveActiveChromeTabToGroup(ctx context.Context, groupName string) error {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil
	}

	snapshot, err := captureChromeTabGroupSnapshot(ctx, groupName)
	if err != nil {
		return err
	}
	if snapshot.selected == nil || snapshot.selected.inGroup {
		return nil
	}
	if snapshot.target() == nil {
		return createChromeTabGroup(ctx, groupName)
	}

	startX, startY := snapshot.selected.center()
	endX, endY := snapshot.target().center()
	return dragChromeTab(ctx, startX, startY, endX, endY)
}

func captureChromeTabGroupSnapshot(ctx context.Context, groupName string) (chromeTabGroupSnapshot, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return chromeTabGroupSnapshot{}, nil
	}

	cmd := exec.CommandContext(ctx, "osascript", "-", groupName)
	cmd.Stdin = strings.NewReader(chromeTabGroupSnapshotScript)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
			return chromeTabGroupSnapshot{}, ctx.Err()
		}
		return chromeTabGroupSnapshot{}, fmt.Errorf("capture chrome tab group snapshot: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	raw := strings.TrimSpace(stdout.String())
	if raw == "" {
		return chromeTabGroupSnapshot{}, nil
	}
	return parseChromeTabGroupSnapshot(raw)
}

func dragChromeTab(ctx context.Context, startX, startY, endX, endY int) error {
	if _, err := exec.LookPath("swift"); err != nil {
		return nil
	}

	cmd := exec.CommandContext(
		ctx,
		"swift",
		"-",
		fmt.Sprintf("%d", startX),
		fmt.Sprintf("%d", startY),
		fmt.Sprintf("%d", endX),
		fmt.Sprintf("%d", endY),
	)
	cmd.Stdin = strings.NewReader(chromeTabGroupDragScript)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("drag chrome tab into group: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func createChromeTabGroup(ctx context.Context, groupName string) error {
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, "osascript", "-", groupName)
	cmd.Stdin = strings.NewReader(chromeTabGroupCreateScript)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("create chrome tab group: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
