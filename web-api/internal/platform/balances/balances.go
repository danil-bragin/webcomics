// Package balances queries provider account/balance endpoints in parallel
// for the dashboard. Each provider is best-effort — failures are surfaced as
// a non-empty Error field per row, never panic.
package balances

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/infrastructure/config"
)

// View is what the HTTP layer returns.
type View struct {
	Providers []Provider `json:"providers"`
}

type Provider struct {
	Name      string  `json:"name"`
	Currency  string  `json:"currency"` // "usd" or "chars"
	Used      float64 `json:"used"`
	Limit     float64 `json:"limit"` // 0 = unbounded/unknown
	Remaining float64 `json:"remaining"`
	UnitLabel string  `json:"unit_label"`
	ResetUnix int64   `json:"reset_unix,omitempty"` // 0 = no scheduled reset
	Error     string  `json:"error,omitempty"`
}

// Client owns API keys + a shared http.Client. Snapshot results are cached
// 15s server-side so the dashboard (which polls every 15s) doesn't pay the
// ~2s provider-RTT cost on every page load or refocus.
type Client struct {
	cfg  *config.Config
	http *http.Client

	mu       sync.Mutex
	cached   View
	cachedAt time.Time
}

const balanceCacheTTL = 15 * time.Second

func New(i do.Injector) (*Client, error) {
	cfg := do.MustInvoke[*config.Config](i)
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 8 * time.Second},
	}, nil
}

func (c *Client) HealthCheck() error { return nil }
func (c *Client) Shutdown() error    { return nil }

// Snapshot fetches every provider concurrently. Result is cached for
// balanceCacheTTL so the dashboard doesn't refire 3 provider HTTP calls per
// page render.
func (c *Client) Snapshot(ctx context.Context) View {
	c.mu.Lock()
	if !c.cachedAt.IsZero() && time.Since(c.cachedAt) < balanceCacheTTL {
		v := c.cached
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()

	out := View{Providers: make([]Provider, 3)}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); out.Providers[0] = c.openrouter(ctx) }()
	go func() { defer wg.Done(); out.Providers[1] = c.fal(ctx) }()
	go func() { defer wg.Done(); out.Providers[2] = c.elevenlabs(ctx) }()
	wg.Wait()

	c.mu.Lock()
	c.cached = out
	c.cachedAt = time.Now()
	c.mu.Unlock()
	return out
}

// --- OpenRouter ---

func (c *Client) openrouter(ctx context.Context) Provider {
	p := Provider{Name: "openrouter", Currency: "usd", UnitLabel: "USD"}
	key := c.cfg.OpenRouterAPIKey
	if key == "" {
		p.Error = "OPENROUTER_API_KEY not set"
		return p
	}
	var body struct {
		Data struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "https://openrouter.ai/api/v1/credits",
		map[string]string{"Authorization": "Bearer " + key}, &body); err != nil {
		p.Error = err.Error()
		return p
	}
	p.Limit = body.Data.TotalCredits
	p.Used = body.Data.TotalUsage
	p.Remaining = body.Data.TotalCredits - body.Data.TotalUsage
	return p
}

// --- fal.ai ---

func (c *Client) fal(ctx context.Context) Provider {
	p := Provider{Name: "fal", Currency: "usd", UnitLabel: "USD"}
	key := c.cfg.FalAPIKey
	if key == "" {
		p.Error = "FAL_KEY not set"
		return p
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://rest.alpha.fal.ai/billing/user_balance", nil)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	req.Header.Set("Authorization", "Key "+key)
	resp, err := c.http.Do(req)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		p.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return p
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	// Endpoint returns a bare number.
	val, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		p.Error = "parse balance: " + err.Error()
		return p
	}
	p.Remaining = val
	// fal doesn't expose a limit / usage split here — only the remaining
	// balance. Limit and Used stay zero so the UI knows it's pure remaining.
	return p
}

// --- ElevenLabs ---

func (c *Client) elevenlabs(ctx context.Context) Provider {
	p := Provider{Name: "elevenlabs", Currency: "chars", UnitLabel: "characters"}
	key := c.cfg.ElevenLabsAPIKey
	if key == "" {
		p.Error = "ELEVENLABS_API_KEY not set"
		return p
	}
	var body struct {
		CharacterCount              int64 `json:"character_count"`
		CharacterLimit              int64 `json:"character_limit"`
		NextCharacterCountResetUnix int64 `json:"next_character_count_reset_unix"`
	}
	if err := c.getJSON(ctx, "https://api.elevenlabs.io/v1/user/subscription",
		map[string]string{"xi-api-key": key}, &body); err != nil {
		p.Error = err.Error()
		return p
	}
	p.Used = float64(body.CharacterCount)
	p.Limit = float64(body.CharacterLimit)
	p.Remaining = float64(body.CharacterLimit - body.CharacterCount)
	p.ResetUnix = body.NextCharacterCountResetUnix
	return p
}

func (c *Client) getJSON(ctx context.Context, url string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return errors.New("HTTP " + http.StatusText(resp.StatusCode))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
