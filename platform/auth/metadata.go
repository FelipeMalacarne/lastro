package auth

import "net/http"

// MetadataPath is where OAuth clients find how to get a token for lastro
// (RFC 9728). The MCP endpoint has its own document at MetadataPath + "/mcp".
const MetadataPath = "/.well-known/oauth-protected-resource"

// ResourceMetadata is the protected resource metadata (RFC 9728).
type ResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceName           string   `json:"resource_name,omitempty"`
	// CLIClientID is the public client `lastro connect` uses, so the CLI only
	// needs the server's URL. An extension; RFC 9728 allows them.
	CLIClientID string `json:"lastro_cli_client_id,omitempty"`
}

// ProtectedResource serves the metadata. It is public: it says where to sign
// in, nothing about the owner.
func ProtectedResource(m ResourceMetadata) http.Handler {
	if m.BearerMethodsSupported == nil {
		m.BearerMethodsSupported = []string{"header"}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		writeJSON(w, http.StatusOK, m)
	})
}
