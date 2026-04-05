package agentmessage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	cliName             = "agent-message"
	maxUsernameLength   = 32
	chatUsernamePrefix  = "cva-"
	defaultRegisterPINs = 6
)

var (
	ErrNoMasterRecipient = errors.New("agent-message master recipient is not configured")

	usernameSanitizer = regexp.MustCompile(`[^a-z0-9._-]+`)
	readLinePattern   = regexp.MustCompile(`^\[(\d+)\]\s+(\S+)\s+([^:]+):\s?(.*)$`)
)

type ChatAccount struct {
	ChatID string
	Name   string
	Master string
}

type Service interface {
	WithChatAccount(context.Context, string, func(ChatAccount) error) error
	CatalogPrompt(context.Context, string) (string, error)
	SendJSONRender(context.Context, string, string) error
	ReadReplies(context.Context, string) ([]IncomingMessage, error)
	ReactToMessage(context.Context, string, string, string) error
}

type IncomingMessage struct {
	ID     string
	Sender string
	Text   string
}

type readMessage struct {
	IncomingMessage
	Index int
}

type commandRunner interface {
	Run(context.Context, ...string) (string, error)
}

type Client struct {
	mu     sync.Mutex
	runner commandRunner
	cache  map[string][]readMessage
}

func NewClient() *Client {
	return &Client{
		runner: execRunner{},
		cache:  map[string][]readMessage{},
	}
}

func NewClientWithRunner(runner commandRunner) *Client {
	if runner == nil {
		runner = execRunner{}
	}
	return &Client{
		runner: runner,
		cache:  map[string][]readMessage{},
	}
}

func DeriveChatAccountName(chatID string) string {
	sanitized := usernameSanitizer.ReplaceAllString(strings.ToLower(strings.TrimSpace(chatID)), "-")
	sanitized = strings.Trim(sanitized, "-.")
	if sanitized == "" {
		sanitized = "chat"
	}

	candidate := chatUsernamePrefix + sanitized
	if len(candidate) <= maxUsernameLength {
		return candidate
	}

	sum := sha256.Sum256([]byte(chatID))
	suffix := hex.EncodeToString(sum[:])[:10]
	available := maxUsernameLength - len(chatUsernamePrefix) - len(suffix) - 1
	if available < 3 {
		available = 3
	}
	if len(sanitized) > available {
		sanitized = sanitized[:available]
	}
	sanitized = strings.Trim(sanitized, "-.")
	if sanitized == "" {
		sanitized = "chat"
	}
	return fmt.Sprintf("%s%s-%s", chatUsernamePrefix, sanitized, suffix)
}

func (c *Client) WithChatAccount(ctx context.Context, chatID string, fn func(ChatAccount) error) error {
	if fn == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	account, err := c.ensureChatAccountLocked(ctx, chatID)
	if err != nil {
		return err
	}
	return fn(account)
}

func (c *Client) CatalogPrompt(ctx context.Context, chatID string) (string, error) {
	var prompt string
	err := c.WithChatAccount(ctx, chatID, func(ChatAccount) error {
		output, err := c.runner.Run(ctx, "catalog", "prompt")
		if err != nil {
			return err
		}
		prompt = strings.TrimSpace(output)
		return nil
	})
	return prompt, err
}

func (c *Client) SendJSONRender(ctx context.Context, chatID string, payload string) error {
	return c.WithChatAccount(ctx, chatID, func(account ChatAccount) error {
		if strings.TrimSpace(account.Master) == "" {
			return ErrNoMasterRecipient
		}
		_, err := c.runner.Run(ctx, "send", payload, "--kind", "json_render")
		return err
	})
}

func (c *Client) ReadReplies(ctx context.Context, chatID string) ([]IncomingMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	account, err := c.ensureChatAccountLocked(ctx, chatID)
	if err != nil {
		if errors.Is(err, ErrNoMasterRecipient) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(account.Master) == "" {
		return nil, nil
	}

	output, err := c.runner.Run(ctx, "read", account.Master, "--n", "50")
	if err != nil {
		return nil, err
	}
	messages := parseReadOutput(output)
	c.cache[account.ChatID] = messages

	replies := make([]IncomingMessage, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.Sender) != account.Master {
			continue
		}
		text := strings.TrimSpace(message.Text)
		if text == "" || text == "[json-render]" || text == "deleted message" {
			continue
		}
		replies = append(replies, message.IncomingMessage)
	}
	return replies, nil
}

func (c *Client) ReactToMessage(ctx context.Context, chatID, messageID, emoji string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.TrimSpace(messageID) == "" || strings.TrimSpace(emoji) == "" {
		return nil
	}
	if _, err := c.ensureChatAccountLocked(ctx, chatID); err != nil {
		return err
	}
	index, ok := c.cachedMessageIndexLocked(chatID, messageID)
	if !ok {
		return fmt.Errorf("agent-message message %s is not in the latest read cache for chat %s", messageID, chatID)
	}
	_, err := c.runner.Run(ctx, "react", strconv.Itoa(index), emoji)
	return err
}

func (c *Client) ensureChatAccountLocked(ctx context.Context, chatID string) (ChatAccount, error) {
	account := ChatAccount{
		ChatID: strings.TrimSpace(chatID),
		Name:   DeriveChatAccountName(chatID),
	}
	if account.ChatID == "" {
		return ChatAccount{}, errors.New("agent-message chat id is required")
	}

	preservedMaster, _ := c.currentMasterLocked(ctx)
	if _, err := c.runner.Run(ctx, "profile", "switch", account.Name); err != nil {
		pin, pinErr := newRegisterPIN()
		if pinErr != nil {
			return ChatAccount{}, pinErr
		}
		if _, registerErr := c.runner.Run(ctx, "register", account.Name, pin); registerErr != nil {
			return ChatAccount{}, fmt.Errorf("agent-message ensure chat account %s: %w", account.Name, registerErr)
		}
	}

	currentMaster, err := c.currentMasterLocked(ctx)
	if err != nil {
		return ChatAccount{}, err
	}
	if currentMaster == "" && preservedMaster != "" {
		if _, err := c.runner.Run(ctx, "config", "set", "master", preservedMaster); err != nil {
			return ChatAccount{}, err
		}
		currentMaster = preservedMaster
	}
	account.Master = currentMaster
	return account, nil
}

func (c *Client) cachedMessageIndexLocked(chatID, messageID string) (int, bool) {
	for _, message := range c.cache[strings.TrimSpace(chatID)] {
		if message.ID == strings.TrimSpace(messageID) {
			return message.Index, true
		}
	}
	return 0, false
}

func parseReadOutput(raw string) []readMessage {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	messages := make([]readMessage, 0, len(lines))
	for _, line := range lines {
		match := readLinePattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 5 {
			continue
		}
		index, err := strconv.Atoi(match[1])
		if err != nil || index <= 0 {
			continue
		}
		messages = append(messages, readMessage{
			IncomingMessage: IncomingMessage{
				ID:     strings.TrimSpace(match[2]),
				Sender: strings.TrimSpace(match[3]),
				Text:   strings.TrimSpace(match[4]),
			},
			Index: index,
		})
	}
	return messages
}

func (c *Client) currentMasterLocked(ctx context.Context) (string, error) {
	output, err := c.runner.Run(ctx, "config", "get", "master")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func newRegisterPIN() (string, error) {
	buf := make([]byte, defaultRegisterPINs)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("agent-message generate register pin: %w", err)
	}

	digits := make([]byte, defaultRegisterPINs)
	for idx, value := range buf {
		digits[idx] = byte('0' + (value % 10))
	}
	if digits[0] == '0' {
		digits[0] = '1'
	}
	return string(digits), nil
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cliName, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s %s: %s", cliName, strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}
