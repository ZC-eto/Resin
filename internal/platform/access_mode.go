package platform

import "strings"

type ProxyAccessMode string

const (
	ProxyAccessModeStandard ProxyAccessMode = "STANDARD"
	ProxyAccessModeSticky   ProxyAccessMode = "STICKY"
)

func NormalizeProxyAccessMode(raw string) ProxyAccessMode {
	switch ProxyAccessMode(strings.ToUpper(strings.TrimSpace(raw))) {
	case "":
		return ProxyAccessModeStandard
	case ProxyAccessModeStandard:
		return ProxyAccessModeStandard
	case ProxyAccessModeSticky:
		return ProxyAccessModeSticky
	default:
		return ""
	}
}

func (m ProxyAccessMode) IsValid() bool {
	return m == ProxyAccessModeStandard || m == ProxyAccessModeSticky
}
