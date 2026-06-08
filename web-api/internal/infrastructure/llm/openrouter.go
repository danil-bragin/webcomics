// Package llm is a tiny OpenRouter chat client used for synchronous,
// request-time text generation (e.g. viral upload metadata). The heavy
// pipeline LLM work lives in the Python workers; this is only for short,
// interactive calls the API serves directly.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

// Client is a minimal OpenRouter chat-completions caller.
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// New returns a client. Empty model falls back to a cheap, capable default.
func New(apiKey, model string) *Client {
	if model == "" {
		model = "openai/gpt-4o"
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		model:   model,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Enabled reports whether an API key is configured.
func (c *Client) Enabled() bool { return c != nil && c.apiKey != "" }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// CompleteJSON sends system+user prompts asking for a JSON object and returns
// the raw JSON content string. Uses response_format=json_object so the model
// is constrained to valid JSON.
func (c *Client) CompleteJSON(ctx context.Context, system, user string, temperature float64) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("openrouter: no API key configured")
	}
	body, _ := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature:    temperature,
		ResponseFormat: map[string]any{"type": "json_object"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(raw))
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("openrouter: bad response: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("openrouter: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("openrouter: empty choices")
	}
	return cr.Choices[0].Message.Content, nil
}
