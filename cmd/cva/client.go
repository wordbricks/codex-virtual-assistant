package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

type createRunRequest struct {
	UserRequestRaw        string `json:"user_request_raw"`
	MaxGenerationAttempts int    `json:"max_generation_attempts,omitempty"`
	ParentRunID           string `json:"parent_run_id,omitempty"`
}

type createRunResponse struct {
	Run       assistant.Run `json:"run"`
	ChatURL   string        `json:"chat_url"`
	StatusURL string        `json:"status_url"`
	EventsURL string        `json:"events_url"`
}

type chatsResponse struct {
	Chats []assistant.Chat `json:"chats"`
}

type scheduledRunsResponse struct {
	ScheduledRuns []assistant.ScheduledRun `json:"scheduled_runs"`
}

func (c *Client) CreateRun(ctx context.Context, request string, maxAttempts int, parentRunID string) (*createRunResponse, error) {
	body := createRunRequest{
		UserRequestRaw:        request,
		MaxGenerationAttempts: maxAttempts,
		ParentRunID:           parentRunID,
	}
	var resp createRunResponse
	if err := c.post(ctx, "/api/v1/runs", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (*store.RunRecord, error) {
	var resp store.RunRecord
	if err := c.get(ctx, "/api/v1/runs/"+runID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListChats(ctx context.Context) ([]assistant.Chat, error) {
	var resp chatsResponse
	if err := c.get(ctx, "/api/v1/chats", &resp); err != nil {
		return nil, err
	}
	return resp.Chats, nil
}

func (c *Client) GetChat(ctx context.Context, chatID string) (*store.ChatRecord, error) {
	var resp store.ChatRecord
	if err := c.get(ctx, "/api/v1/chats/"+chatID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CancelRun(ctx context.Context, runID string) (*store.RunRecord, error) {
	var resp store.RunRecord
	if err := c.post(ctx, "/api/v1/runs/"+runID+"/cancel", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ResumeRun(ctx context.Context, runID string, input map[string]string) error {
	body := struct {
		Input map[string]string `json:"input"`
	}{Input: input}
	var resp json.RawMessage
	return c.post(ctx, "/api/v1/runs/"+runID+"/resume", body, &resp)
}

func (c *Client) ListScheduledRuns(ctx context.Context, chatID string, status assistant.ScheduledRunStatus) ([]assistant.ScheduledRun, error) {
	path := "/api/v1/scheduled"
	query := make([]string, 0, 2)
	if chatID != "" {
		query = append(query, "chat_id="+chatID)
	}
	if status != "" {
		query = append(query, "status="+string(status))
	}
	if len(query) > 0 {
		path += "?" + strings.Join(query, "&")
	}
	var resp scheduledRunsResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.ScheduledRuns, nil
}

func (c *Client) GetScheduledRun(ctx context.Context, scheduledRunID string) (*assistant.ScheduledRun, error) {
	var resp assistant.ScheduledRun
	if err := c.get(ctx, "/api/v1/scheduled/"+scheduledRunID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateScheduledRun(ctx context.Context, runID, scheduledFor, prompt string, maxAttempts int) (*assistant.ScheduledRun, error) {
	body := struct {
		ScheduledFor          string `json:"scheduled_for"`
		Prompt                string `json:"prompt"`
		MaxGenerationAttempts int    `json:"max_generation_attempts,omitempty"`
	}{
		ScheduledFor:          scheduledFor,
		Prompt:                prompt,
		MaxGenerationAttempts: maxAttempts,
	}
	var resp assistant.ScheduledRun
	if err := c.post(ctx, "/api/v1/runs/"+runID+"/scheduled", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) UpdateScheduledRun(ctx context.Context, scheduledRunID, scheduledFor, prompt string, maxAttempts int) (*assistant.ScheduledRun, error) {
	body := struct {
		ScheduledFor          string `json:"scheduled_for,omitempty"`
		Prompt                string `json:"prompt,omitempty"`
		MaxGenerationAttempts int    `json:"max_generation_attempts,omitempty"`
	}{
		ScheduledFor:          scheduledFor,
		Prompt:                prompt,
		MaxGenerationAttempts: maxAttempts,
	}
	var resp assistant.ScheduledRun
	if err := c.post(ctx, "/api/v1/scheduled/"+scheduledRunID+"/update", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CancelScheduledRun(ctx context.Context, scheduledRunID string) (*assistant.ScheduledRun, error) {
	var resp assistant.ScheduledRun
	if err := c.post(ctx, "/api/v1/scheduled/"+scheduledRunID+"/cancel", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) StreamEvents(ctx context.Context, runID string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/runs/"+runID+"/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("SSE stream returned %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
