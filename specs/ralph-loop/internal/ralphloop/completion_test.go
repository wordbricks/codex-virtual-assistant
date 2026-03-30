package ralphloop

import "testing"

func TestContainsCompletionSignal(t *testing.T) {
	if !containsCompletionSignal("done <promise>COMPLETE</promise>") {
		t.Fatal("expected completion signal to be detected")
	}
	if containsCompletionSignal("done") {
		t.Fatal("did not expect completion signal")
	}
}

func TestCollectAgentText(t *testing.T) {
	got := collectAgentText([]string{" hello ", "", "world "})
	if got != "hello\nworld" {
		t.Fatalf("collectAgentText() = %q", got)
	}
}
