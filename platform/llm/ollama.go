// Package llm is the client for Ollama, the local LLM running on zaros.
//
// Modules do not depend on this package directly: each module declares the
// minimal interface it needs (e.g. a Classifier in categorize) and the
// composition root injects this client.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrDisabled means the LLM is not configured.
var ErrDisabled = errors.New("llm: disabled (set LASTRO_OLLAMA_URL and LASTRO_OLLAMA_MODEL)")

type Client struct {
	baseURL string
	model   string
	http    *http.Client
}

func NewOllama(baseURL, model string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{Timeout: 2 * time.Minute},
	}
}

// Model is the model's name, as configured.
func (c *Client) Model() string { return c.model }

// Enabled reports whether a URL and a model are configured.
func (c *Client) Enabled() bool { return c.baseURL != "" && c.model != "" }

type generateRequest struct {
	Model  string `json:"model"`
	System string `json:"system,omitempty"`
	Prompt string `json:"prompt"`
	Format string `json:"format,omitempty"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

// Generate returns the response as text.
func (c *Client) Generate(ctx context.Context, system, prompt string) (string, error) {
	return c.generate(ctx, system, prompt, "")
}

// GenerateJSON asks for a JSON response and decodes it into out.
func (c *Client) GenerateJSON(ctx context.Context, system, prompt string, out any) error {
	raw, err := c.generate(ctx, system, prompt, "json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("llm: response is not valid JSON: %w", err)
	}
	return nil
}

func (c *Client) generate(ctx context.Context, system, prompt, format string) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	body, err := json.Marshal(generateRequest{Model: c.model, System: system, Prompt: prompt, Format: format})
	if err != nil {
		return "", fmt.Errorf("llm: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("llm: status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("llm: decode response: %w", err)
	}
	return out.Response, nil
}

// ErrModelMissing means Ollama answers but does not have the configured model.
var ErrModelMissing = errors.New("llm: the model is not pulled in Ollama")

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Ping checks that Ollama answers and has the configured model, without
// generating anything.
func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("llm: status %d", resp.StatusCode)
	}
	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("llm: decode tags: %w", err)
	}
	for _, m := range tags.Models {
		// "llama3.1" is pulled as "llama3.1:latest".
		if m.Name == c.model || strings.TrimSuffix(m.Name, ":latest") == c.model {
			return nil
		}
	}
	return ErrModelMissing
}
