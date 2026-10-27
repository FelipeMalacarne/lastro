package app

import (
	"errors"
	"net/http"
	"net/url"

	"lastro/platform/httpx"
	"lastro/platform/llm"
)

// settingsJSON is what the Settings screen shows about the server. Every
// value comes from the LASTRO_* variables, so it is read-only here.
type settingsJSON struct {
	Version string  `json:"version"`
	Env     string  `json:"env"`
	SignIn  string  `json:"signIn"`
	Model   modelJS `json:"model"`
	// Alerts is where new alerts are pushed: "Telegram", or "" when only
	// the app shows them (ADR-0007).
	Alerts string `json:"alerts"`
}

type modelJS struct {
	// State is "off" (not configured), "online", "missing" (Ollama answers
	// without the model) or "offline".
	State string `json:"state"`
	Name  string `json:"name"`
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	out := settingsJSON{Version: a.Version, Env: a.Config.Env, SignIn: "Off (dev)"}
	if u, err := url.Parse(a.Config.Auth.OIDC.Issuer); !a.Config.Auth.Disabled && err == nil {
		out.SignIn = u.Host
	}
	if a.Notify.Enabled() {
		out.Alerts = "Telegram"
	}
	out.Model.Name = a.LLM.Model()
	switch err := a.LLM.Ping(r.Context()); {
	case err == nil:
		out.Model.State = "online"
	case errors.Is(err, llm.ErrDisabled):
		out.Model.State = "off"
	case errors.Is(err, llm.ErrModelMissing):
		out.Model.State = "missing"
	default:
		a.Logger.WarnContext(r.Context(), "local model unreachable", "error", err)
		out.Model.State = "offline"
	}
	httpx.JSON(w, http.StatusOK, out)
}
