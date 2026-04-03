package agentmessage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestDeriveChatAccountNameIsDeterministicAndBounded(t *testing.T) {
	t.Parallel()

	name := DeriveChatAccountName("chat_kix5xq_0123456789abcdef")
	if !strings.HasPrefix(name, "cva-") {
		t.Fatalf("name = %q, want cva- prefix", name)
	}
	if len(name) > maxUsernameLength {
		t.Fatalf("len(name) = %d, want <= %d", len(name), maxUsernameLength)
	}
	if got := DeriveChatAccountName("chat_kix5xq_0123456789abcdef"); got != name {
		t.Fatalf("DeriveChatAccountName() = %q, want %q", got, name)
	}
}

func TestSendJSONRenderCreatesAndReusesChatAccount(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		master: "supervisor",
	}
	client := NewClientWithRunner(runner)

	payload := `{"root":"screen","elements":{"screen":{"type":"Text","props":{"text":"done"},"children":[]}}}`
	if err := client.SendJSONRender(context.Background(), "chat_abc123", payload); err != nil {
		t.Fatalf("first SendJSONRender() error = %v", err)
	}
	if err := client.SendJSONRender(context.Background(), "chat_abc123", payload); err != nil {
		t.Fatalf("second SendJSONRender() error = %v", err)
	}

	if runner.registers != 1 {
		t.Fatalf("registers = %d, want 1", runner.registers)
	}
	if runner.sends != 2 {
		t.Fatalf("sends = %d, want 2", runner.sends)
	}
}

func TestRenderLifecycleCardBuildsJSONSpec(t *testing.T) {
	t.Parallel()

	payload, err := RenderLifecycleCard(CompletedCard(assistant.Run{
		ID: "run_123",
		LatestEvaluation: &assistant.Evaluation{
			Summary: "Delivered a concise final report.",
		},
	}))
	if err != nil {
		t.Fatalf("RenderLifecycleCard() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded["root"] == "" {
		t.Fatalf("payload = %#v, want root", decoded)
	}
	if _, ok := decoded["elements"].(map[string]any); !ok {
		t.Fatalf("elements type = %T, want map[string]any", decoded["elements"])
	}
}

type fakeRunner struct {
	mu        sync.Mutex
	master    string
	profiles  map[string]bool
	registers int
	sends     int
}

func (r *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.profiles == nil {
		r.profiles = map[string]bool{}
	}

	switch strings.Join(args, " ") {
	case "config get master":
		return r.master + "\n", nil
	}

	if len(args) >= 3 && args[0] == "profile" && args[1] == "switch" {
		if r.profiles[args[2]] {
			return "switched\n", nil
		}
		return "", errors.New("profile not found")
	}
	if len(args) >= 3 && args[0] == "register" {
		r.profiles[args[1]] = true
		r.registers++
		r.master = ""
		return "registered\n", nil
	}
	if len(args) == 4 && args[0] == "config" && args[1] == "set" && args[2] == "master" {
		r.master = args[3]
		return r.master + "\n", nil
	}
	if len(args) >= 4 && args[0] == "send" {
		r.sends++
		return "sent\n", nil
	}
	return "", nil
}
