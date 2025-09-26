package model

import (
	"encoding/json"
	"strings"
)

// InboundProxySettings represents the parsed proxySettings payload stored on an inbound.
// Historically this field contained the raw Xray proxySettings object. Newer versions repurpose
// the field to store higher level metadata such as enabling Tor upstream routing for clients.
type InboundProxySettings struct {
	Tag            string            `json:"tag,omitempty"`
	TransportLayer *bool             `json:"transportLayer,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	Tor            *TorProxySettings `json:"tor,omitempty"`
}

// TorProxySettings holds Tor specific configuration metadata persisted for an inbound.
type TorProxySettings struct {
	Enabled   bool   `json:"enabled"`
	Isolation string `json:"isolation,omitempty"`
}

const (
	// TorIsolationPerClient represents the isolation strategy where every client gets its own
	// isolated Tor circuit via SOCKS authentication separation.
	TorIsolationPerClient = "per-client"
	torModeIdentifier     = "tor"
)

// ParseProxySettings parses a raw JSON string that may hold either legacy Xray proxy settings or the
// newer metadata based representation. Invalid or empty payloads yield a zero-value structure.
func ParseProxySettings(raw string) InboundProxySettings {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return InboundProxySettings{}
	}
	var ps InboundProxySettings
	if err := json.Unmarshal([]byte(raw), &ps); err != nil {
		// Legacy payloads may simply be the JSON object required by Xray. Attempt to unmarshal
		// into the legacy shape before giving up.
		legacy := struct {
			Tag            string `json:"tag"`
			TransportLayer bool   `json:"transportLayer"`
		}{}
		if err2 := json.Unmarshal([]byte(raw), &legacy); err2 == nil {
			ps.Tag = legacy.Tag
			ps.TransportLayer = &legacy.TransportLayer
		}
	}
	return ps
}

// IsLegacyTag returns true when the proxy settings still carry the legacy tag based structure used
// by Xray. In this case the returned object should be forwarded to the generated Xray config.
func (ps InboundProxySettings) IsLegacyTag() bool {
	return ps.Tag != ""
}

// IsTorPerClientEnabled returns true when Tor upstream routing is enabled with per-client isolation.
func (ps InboundProxySettings) IsTorPerClientEnabled() bool {
	if ps.Tor == nil {
		return false
	}
	if !ps.Tor.Enabled {
		return false
	}
	if ps.Mode != "" && !strings.EqualFold(ps.Mode, torModeIdentifier) {
		return false
	}
	if ps.Tor.Isolation == "" {
		return true
	}
	return strings.EqualFold(ps.Tor.Isolation, TorIsolationPerClient)
}
