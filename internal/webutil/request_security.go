package webutil

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func NetworkPrefix(ipText string) string {
	ip := net.ParseIP(strings.TrimSpace(ipText))
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String() + "/24"
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func requestSecurityHashWithSecret(secret, kind, value string) string {
	secret = strings.TrimSpace(secret)
	value = strings.TrimSpace(value)
	if secret == "" || value == "" {
		return ""
	}
	return cloud.HashSecret(secret + "\x00request-security-v1\x00" + kind + "\x00" + value)
}

func requestSecuritySecrets() (primary, legacy string) {
	legacy = strings.TrimSpace(os.Getenv("MCP_AUTH_SECRET"))
	primary = strings.TrimSpace(os.Getenv("CODELOCAL_SECURITY_BINDING_SECRET"))
	if primary == "" {
		return legacy, ""
	}
	if primary == legacy {
		legacy = ""
	}
	return primary, legacy
}

func normalizeSecurityAgent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	lastDigit := false
	for _, char := range value {
		if char >= '0' && char <= '9' {
			if !lastDigit {
				out.WriteByte('#')
			}
			lastDigit = true
			continue
		}
		lastDigit = false
		out.WriteRune(char)
		if out.Len() >= 512 {
			break
		}
	}
	return out.String()
}

func RequestSecuritySignal(r *http.Request, deviceToken string) cloud.SecuritySignal {
	primary, legacy := requestSecuritySecrets()
	agent := normalizeSecurityAgent(r.UserAgent())
	network := NetworkPrefix(ClientIP(r))
	return cloud.SecuritySignal{
		DeviceHash:        requestSecurityHashWithSecret(primary, "device", deviceToken),
		AgentHash:         requestSecurityHashWithSecret(primary, "agent", agent),
		NetworkHash:       requestSecurityHashWithSecret(primary, "network", network),
		LegacyDeviceHash:  requestSecurityHashWithSecret(legacy, "device", deviceToken),
		LegacyAgentHash:   requestSecurityHashWithSecret(legacy, "agent", agent),
		LegacyNetworkHash: requestSecurityHashWithSecret(legacy, "network", network),
	}
}
