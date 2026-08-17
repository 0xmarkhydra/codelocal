package cloud

import (
	"context"
	"strings"
	"time"
)

type SecuritySignal struct {
	DeviceHash        string `json:"deviceHash,omitempty"`
	AgentHash         string `json:"agentHash,omitempty"`
	NetworkHash       string `json:"networkHash,omitempty"`
	LegacyDeviceHash  string `json:"-"`
	LegacyAgentHash   string `json:"-"`
	LegacyNetworkHash string `json:"-"`
}

type SecurityDecision struct {
	FirstSeen      bool
	DeviceMismatch bool
	AgentChanged   bool
	NetworkChanged bool
	HashUpgrade    bool
	HighRisk       bool
}

func securityHashMatches(previous, current, legacy string) bool {
	return previous != "" && (previous == current || (legacy != "" && previous == legacy))
}

func securityHashUpgrade(previous, current, legacy string) bool {
	return legacy != "" && previous == legacy && previous != current
}

func EvaluateSecuritySignals(previous, current SecuritySignal) SecurityDecision {
	deviceMismatch := previous.DeviceHash != "" && !securityHashMatches(previous.DeviceHash, current.DeviceHash, current.LegacyDeviceHash)
	agentChanged := previous.AgentHash != "" && current.AgentHash != "" && !securityHashMatches(previous.AgentHash, current.AgentHash, current.LegacyAgentHash)
	networkChanged := previous.NetworkHash != "" && current.NetworkHash != "" && !securityHashMatches(previous.NetworkHash, current.NetworkHash, current.LegacyNetworkHash)
	hashUpgrade := securityHashUpgrade(previous.DeviceHash, current.DeviceHash, current.LegacyDeviceHash) || securityHashUpgrade(previous.AgentHash, current.AgentHash, current.LegacyAgentHash) || securityHashUpgrade(previous.NetworkHash, current.NetworkHash, current.LegacyNetworkHash)
	return SecurityDecision{
		DeviceMismatch: deviceMismatch,
		AgentChanged:   agentChanged,
		NetworkChanged: networkChanged,
		HashUpgrade:    hashUpgrade,
		HighRisk:       deviceMismatch,
	}
}

func securityStateScope(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "session" || value == "credential" {
		return value
	}
	return "generic"
}

func securityStateKey(scope, subject string) string {
	return "codelocal:security:" + securityStateScope(scope) + ":" + HashSecret(subject)[:32]
}

func (s *Store) ClearSecurityState(ctx context.Context, scope, subject string) error {
	if s == nil || s.Redis == nil || strings.TrimSpace(subject) == "" {
		return nil
	}
	return s.Redis.Del(ctx, securityStateKey(scope, subject)).Err()
}

func (s *Store) ObserveSecurityState(ctx context.Context, scope, subject string, signal SecuritySignal, ttl time.Duration) (SecurityDecision, error) {
	if s == nil || s.Redis == nil || strings.TrimSpace(subject) == "" {
		return SecurityDecision{}, nil
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	const script = `
local exists=redis.call('EXISTS',KEYS[1])
local device=ARGV[1]
local agent=ARGV[2]
local network=ARGV[3]
local now=ARGV[4]
local ttl=ARGV[5]
if exists==0 then
 redis.call('HSET',KEYS[1],'device',device,'agent',agent,'network',network,'firstSeenAt',now,'lastSeenAt',now,'riskAt','0')
 redis.call('PEXPIRE',KEYS[1],ttl)
 return {1,0,0,0,0}
end
local previousDevice=redis.call('HGET',KEYS[1],'device') or ''
local previousAgent=redis.call('HGET',KEYS[1],'agent') or ''
local previousNetwork=redis.call('HGET',KEYS[1],'network') or ''
local deviceMismatch=0
local agentChanged=0
local networkChanged=0
if previousDevice~='' and previousDevice~=device then deviceMismatch=1 end
if previousAgent~='' and agent~='' and previousAgent~=agent then agentChanged=1 end
if previousNetwork~='' and network~='' and previousNetwork~=network then networkChanged=1 end
local highRisk=0
if deviceMismatch==1 then highRisk=1 end
redis.call('HSET',KEYS[1],'lastSeenAt',now)
if highRisk==1 then
 redis.call('HSET',KEYS[1],'riskAt',now)
else
 if device~='' then redis.call('HSET',KEYS[1],'device',device) end
 if agent~='' then redis.call('HSET',KEYS[1],'agent',agent) end
 if network~='' then redis.call('HSET',KEYS[1],'network',network) end
end
redis.call('PEXPIRE',KEYS[1],ttl)
return {0,deviceMismatch,agentChanged,networkChanged,highRisk}
`
	values, err := s.Redis.Eval(ctx, script, []string{securityStateKey(scope, subject)}, signal.DeviceHash, signal.AgentHash, signal.NetworkHash, time.Now().UnixMilli(), ttl.Milliseconds()).Int64Slice()
	if err != nil {
		return SecurityDecision{}, err
	}
	if len(values) != 5 {
		return SecurityDecision{}, nil
	}
	return SecurityDecision{
		FirstSeen:      values[0] == 1,
		DeviceMismatch: values[1] == 1,
		AgentChanged:   values[2] == 1,
		NetworkChanged: values[3] == 1,
		HighRisk:       values[4] == 1,
	}, nil
}
