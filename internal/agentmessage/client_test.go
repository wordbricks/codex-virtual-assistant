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

func TestReadRepliesFiltersIncomingMasterMessages(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		master: "supervisor",
		readOutput: strings.Join([]string{
			"[1] msg_1 cva-chat_abc: [json-render]",
			"[2] msg_2 supervisor: First follow-up",
			"[3] msg_3 supervisor: deleted message",
			"[4] msg_4 supervisor: Second follow-up",
		}, "\n"),
	}
	client := NewClientWithRunner(runner)

	replies, err := client.ReadReplies(context.Background(), "chat_abc")
	if err != nil {
		t.Fatalf("ReadReplies() error = %v", err)
	}
	if len(replies) != 2 {
		t.Fatalf("len(replies) = %d, want 2", len(replies))
	}
	if replies[0].ID != "msg_2" || replies[0].Text != "First follow-up" {
		t.Fatalf("replies[0] = %#v", replies[0])
	}
	if replies[1].ID != "msg_4" || replies[1].Text != "Second follow-up" {
		t.Fatalf("replies[1] = %#v", replies[1])
	}
}

func TestReactToMessageUsesCachedReadIndex(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		master: "supervisor",
		readOutput: "[1] msg_1 supervisor: Please continue",
	}
	client := NewClientWithRunner(runner)

	if _, err := client.ReadReplies(context.Background(), "chat_abc"); err != nil {
		t.Fatalf("ReadReplies() error = %v", err)
	}
	if err := client.ReactToMessage(context.Background(), "chat_abc", "msg_1", "👀"); err != nil {
		t.Fatalf("ReactToMessage() error = %v", err)
	}
	if got := strings.Join(runner.lastArgs, " "); got != "react 1 👀" {
		t.Fatalf("lastArgs = %q, want react command", got)
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

func TestPhaseChangedCardIncludesPhaseSummary(t *testing.T) {
	t.Parallel()

	payload, err := RenderLifecycleCard(PhaseChangedCard(assistant.Run{
		ID: "run_456",
	}, assistant.RunPhasePlanning, "Planning the task into a structured TaskSpec."))
	if err != nil {
		t.Fatalf("RenderLifecycleCard() error = %v", err)
	}

	if !strings.Contains(payload, "CVA entered planning") {
		t.Fatalf("payload = %q, want phase title", payload)
	}
	if !strings.Contains(payload, "Planning the task into a structured TaskSpec.") {
		t.Fatalf("payload = %q, want summary", payload)
	}
}

func TestStartedCardIncludesUserRequest(t *testing.T) {
	t.Parallel()

	payload, err := RenderLifecycleCard(StartedCard(assistant.Run{
		ID:             "run_789",
		Phase:          assistant.RunPhaseQueued,
		UserRequestRaw: "Research five competitor pricing pages and summarize them.",
	}, "Run created from the user request."))
	if err != nil {
		t.Fatalf("RenderLifecycleCard() error = %v", err)
	}

	if !strings.Contains(payload, "Research five competitor pricing pages and summarize them.") {
		t.Fatalf("payload = %q, want user request", payload)
	}
}

type fakeRunner struct {
	mu        sync.Mutex
	master    string
	profiles  map[string]bool
	registers int
	sends     int
	readOutput string
	lastArgs  []string
}

func (r *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastArgs = append([]string(nil), args...)

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
	if len(args) >= 3 && args[0] == "read" {
		return r.readOutput, nil
	}
	if len(args) == 3 && args[0] == "react" {
		return "reaction added\n", nil
	}
	return "", nil
}
