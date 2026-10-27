package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"lastro/platform/config"
)

func connect(t *testing.T, a *App) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := a.MCPServer().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestMCPToolsOnlyRead(t *testing.T) {
	cs := connect(t, quietApp())
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("%s is not marked read-only", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
	}
	want := []string{"accounts", "budget", "categories", "contribution_plan", "monthly_report", "portfolio", "transactions"}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
}

// Bad input is answered as a tool error the model can read, before any
// module is touched (this App has none).
func TestMCPRejectsBadInput(t *testing.T) {
	cs := connect(t, quietApp())
	for _, tt := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"budget", map[string]any{"month": "October"}, "month must look like"},
		{"monthly_report", map[string]any{"month": "2026-13"}, ""},
		{"transactions", map[string]any{"month": "2026-10", "flow": "sideways"}, "flow must be"},
		{"transactions", map[string]any{"month": "2026-10", "account": "1234"}, "account must be"},
		{"transactions", map[string]any{"month": "2026-10", "category": "food"}, "category must be"},
		{"transactions", map[string]any{"month": "2026-10", "search": strings.Repeat("a", 101)}, "100 characters"},
		{"contribution_plan", map[string]any{"amountCents": -1}, "negative"},
	} {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
		if err != nil {
			t.Fatalf("%s %v: %v", tt.tool, tt.args, err)
		}
		if !res.IsError {
			t.Fatalf("%s %v: want a tool error", tt.tool, tt.args)
		}
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, tt.want) {
			t.Errorf("%s %v: %q does not say %q", tt.tool, tt.args, text, tt.want)
		}
	}
}

// MCP over HTTP, with auth off as in dev: the stateless handler answers an
// initialize with JSON.
func TestMCPOverHTTP(t *testing.T) {
	h := handlerWith(t, config.Auth{Disabled: true})
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"lastro"`) {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}
