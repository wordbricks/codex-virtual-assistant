package wtl

import "testing"

func TestParseChromeTabGroupSnapshotPrefersVisibleGroupTab(t *testing.T) {
	t.Parallel()

	raw := "group-header\tgroup CVA\t12\t353\t216\t26\ttrue\n" +
		"group-tab\tInbox - Part of group CVA\t22\t385\t206\t30\ttrue\n" +
		"selected\tChatGPT\t12\t515\t216\t30\tfalse"

	snapshot, err := parseChromeTabGroupSnapshot(raw)
	if err != nil {
		t.Fatalf("parseChromeTabGroupSnapshot() error = %v", err)
	}
	if snapshot.selected == nil {
		t.Fatal("selected tab = nil, want parsed node")
	}
	if snapshot.selected.inGroup {
		t.Fatal("selected tab inGroup = true, want false")
	}
	target := snapshot.target()
	if target == nil {
		t.Fatal("target = nil, want visible group target")
	}
	if target.kind != "group-tab" {
		t.Fatalf("target.kind = %q, want %q", target.kind, "group-tab")
	}
}

func TestShouldAttemptChromeTabGrouping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		group   string
		command string
		want    bool
	}{
		{
			name:    "open command enabled",
			group:   "CVA",
			command: "pnpm exec agent-browser open https://example.com",
			want:    true,
		},
		{
			name:    "click command enabled",
			group:   "CVA",
			command: "agent-browser click @e12 --new-tab",
			want:    true,
		},
		{
			name:    "snapshot skipped",
			group:   "CVA",
			command: "agent-browser snapshot -i",
			want:    false,
		},
		{
			name:    "empty group skipped",
			group:   "",
			command: "agent-browser open https://example.com",
			want:    false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldAttemptChromeTabGrouping(tc.group, tc.command); got != tc.want {
				t.Fatalf("shouldAttemptChromeTabGrouping(%q, %q) = %v, want %v", tc.group, tc.command, got, tc.want)
			}
		})
	}
}
