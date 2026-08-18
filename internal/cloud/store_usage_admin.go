package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

func (s *Store) Audit(_ AuditEvent) {
	// Cloud audit storage is disabled. Keep call sites as no-ops so runtime,
	// auth and workspace behavior stay unchanged without persisting metadata.
}

func parseUsageInt64(value string) int64 {
	n, _ := strconv.ParseInt(value, 10, 64)
	return n
}

func usageBucketStart(createdAt int64) int64 {
	return (createdAt / 3600000) * 3600000
}

func usageDayStart(createdAt int64) int64 {
	const dayMs = int64(24 * time.Hour / time.Millisecond)
	return (createdAt / dayMs) * dayMs
}

func usageWindowKey(userID, bucket string, startedAt int64) string {
	return "codelocal:usage:" + bucket + ":" + userID + ":" + strconv.FormatInt(startedAt, 10)
}

func mcpActiveKey(userID string) string { return "codelocal:user:mcp-active:" + userID }

func (s *Store) TouchUserMCPActive(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	return s.Redis.Set(ctx, mcpActiveKey(userID), time.Now().UnixMilli(), 5*time.Minute).Err()
}

func (s *Store) IsUserMCPActive(ctx context.Context, userID string) (bool, error) {
	n, err := s.Redis.Exists(ctx, mcpActiveKey(userID)).Result()
	return n > 0, err
}

func (s *Store) UserMCPActiveMap(ctx context.Context, userIDs []string) (map[string]bool, error) {
	commands := make(map[string]*redis.IntCmd, len(userIDs))
	_, err := s.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, userID := range userIDs {
			if userID == "" {
				continue
			}
			commands[userID] = pipe.Exists(ctx, mcpActiveKey(userID))
		}
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make(map[string]bool, len(commands))
	for userID, command := range commands {
		out[userID] = command.Val() > 0
	}
	return out, nil
}

func (s *Store) RecordMCPUsage(ctx context.Context, event MCPUsageEvent) error {
	_ = ctx
	if strings.TrimSpace(event.UserID) == "" {
		return nil
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	if event.Calls <= 0 {
		event.Calls = 1
	}
	select {
	case s.usageQ <- event:
		return nil
	default:
		// Usage is approximate telemetry, never a reason to delay or fail a user
		// tool call. Keep a visible counter instead of falling back to synchronous
		// PostgreSQL writes on the request path.
		dropped := s.usageDropped.Add(1)
		if dropped == 1 || dropped%1000 == 0 {
			slog.Warn("MCP usage local queue full; telemetry event dropped", "dropped", dropped)
		}
		return nil
	}
}

type usageAggregateKey struct {
	UserID      string
	BucketStart int64
}

type queuedUsageAggregate struct {
	BatchID string
	Event   MCPUsageEvent
}

func mergeMCPUsage(target MCPUsageEvent, event MCPUsageEvent) MCPUsageEvent {
	if target.UserID == "" {
		target = event
		target.Calls = 0
		target.InputBytes = 0
		target.OutputBytes = 0
		target.InputTokensEst = 0
		target.OutputTokensEst = 0
	}
	if event.CreatedAt > target.CreatedAt {
		target.CreatedAt = event.CreatedAt
	}
	target.Calls += event.Calls
	target.InputBytes += event.InputBytes
	target.OutputBytes += event.OutputBytes
	target.InputTokensEst += event.InputTokensEst
	target.OutputTokensEst += event.OutputTokensEst
	return target
}

func (s *Store) ensureUsageStreamGroup(ctx context.Context) error {
	err := s.Redis.XGroupCreateMkStream(ctx, usageStreamKey, usageStreamGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

type usageProducerState struct {
	pending           map[usageAggregateKey]queuedUsageAggregate
	lastActivityTouch map[string]time.Time
}

func newUsageProducerState() *usageProducerState {
	return &usageProducerState{pending: map[usageAggregateKey]queuedUsageAggregate{}, lastActivityTouch: map[string]time.Time{}}
}

func (state *usageProducerState) add(event MCPUsageEvent) {
	if event.Calls <= 0 {
		event.Calls = 1
	}
	key := usageAggregateKey{UserID: event.UserID, BucketStart: usageBucketStart(event.CreatedAt)}
	current := state.pending[key]
	if current.BatchID == "" {
		current.BatchID = RandomHex(16)
	}
	current.Event = mergeMCPUsage(current.Event, event)
	state.pending[key] = current
}

func (state *usageProducerState) flush(s *Store, timeout time.Duration) bool {
	if len(state.pending) == 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	pipe := s.Redis.Pipeline()
	now := time.Now()
	touched := []string{}
	maxLen := int64(envInt("CODELOCAL_USAGE_STREAM_MAXLEN", 1_000_000))
	for _, aggregate := range state.pending {
		raw, err := json.Marshal(aggregate.Event)
		if err != nil {
			continue
		}
		pipe.XAdd(ctx, &redis.XAddArgs{Stream: usageStreamKey, MaxLen: maxLen, Approx: true, Values: map[string]any{"batchId": aggregate.BatchID, "event": string(raw)}})
		if last := state.lastActivityTouch[aggregate.Event.UserID]; last.IsZero() || now.Sub(last) >= time.Minute {
			pipe.Set(ctx, mcpActiveKey(aggregate.Event.UserID), now.UnixMilli(), 5*time.Minute)
			touched = append(touched, aggregate.Event.UserID)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("MCP usage Redis stream publish failed", "error", err, "aggregateCount", len(state.pending))
		return false
	}
	for _, userID := range touched {
		state.lastActivityTouch[userID] = now
	}
	clear(state.pending)
	return true
}

func (state *usageProducerState) drain(s *Store) {
	for {
		select {
		case event := <-s.usageQ:
			state.add(event)
		default:
			_ = state.flush(s, 3*time.Second)
			return
		}
	}
}

func (s *Store) usageStreamProducer() {
	defer s.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	state := newUsageProducerState()
	for {
		select {
		case <-s.ctx.Done():
			state.drain(s)
			return
		case event := <-s.usageQ:
			state.add(event)
			if len(state.pending) >= 256 {
				_ = state.flush(s, 3*time.Second)
			}
		case <-ticker.C:
			_ = state.flush(s, 3*time.Second)
		}
	}
}

func (s *Store) refreshUsageConsumerLease(ctx context.Context) (bool, error) {
	const lease = 15 * time.Second
	script := `if redis.call('GET',KEYS[1])==ARGV[1] then redis.call('PEXPIRE',KEYS[1],ARGV[2]); return 1 end; if redis.call('SET',KEYS[1],ARGV[1],'NX','PX',ARGV[2]) then return 1 end; return 0`
	value, err := s.Redis.Eval(ctx, script, []string{usageConsumerLockKey}, s.usageConsumerID, lease.Milliseconds()).Int()
	return value == 1, err
}

func (s *Store) releaseUsageConsumerLease() {
	script := `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.Redis.Eval(ctx, script, []string{usageConsumerLockKey}, s.usageConsumerID).Err()
}

func usageEventFromStream(message redis.XMessage) (string, MCPUsageEvent, error) {
	batchID := strings.TrimSpace(fmt.Sprint(message.Values["batchId"]))
	if batchID == "" || batchID == "<nil>" {
		batchID = message.ID
	}
	var event MCPUsageEvent
	if err := json.Unmarshal([]byte(fmt.Sprint(message.Values["event"])), &event); err != nil {
		return batchID, event, err
	}
	if event.UserID == "" {
		return batchID, event, errors.New("usage event missing userId")
	}
	if event.Calls <= 0 {
		event.Calls = 1
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	return batchID, event, nil
}

type queuedUsageMessage struct {
	messageID string
	batchID   string
	event     MCPUsageEvent
}

func parseUsageMessages(messages []redis.XMessage) ([]queuedUsageMessage, []string) {
	valid := make([]queuedUsageMessage, 0, len(messages))
	ackIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		batchID, event, err := usageEventFromStream(message)
		if err != nil {
			slog.Warn("discarding invalid MCP usage stream event", "messageId", message.ID, "error", err)
			ackIDs = append(ackIDs, message.ID)
			continue
		}
		valid = append(valid, queuedUsageMessage{messageID: message.ID, batchID: batchID, event: event})
	}
	return valid, ackIDs
}

func mergeQueuedUsage(item queuedUsageMessage, dbAggregates map[string]MCPUsageEvent, windowAggregates map[usageAggregateKey]MCPUsageEvent) {
	dbAggregates[item.event.UserID] = mergeMCPUsage(dbAggregates[item.event.UserID], item.event)
	key := usageAggregateKey{UserID: item.event.UserID, BucketStart: usageBucketStart(item.event.CreatedAt)}
	windowAggregates[key] = mergeMCPUsage(windowAggregates[key], item.event)
}

func persistUsageAggregate(ctx context.Context, tx pgx.Tx, event MCPUsageEvent) error {
	_, err := tx.Exec(ctx, `INSERT INTO codelocal_mcp_usage(user_id,calls,input_bytes,output_bytes,input_tokens_est,output_tokens_est,last_used_at)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(user_id) DO UPDATE SET
 calls=codelocal_mcp_usage.calls+EXCLUDED.calls,
 input_bytes=codelocal_mcp_usage.input_bytes+EXCLUDED.input_bytes,
 output_bytes=codelocal_mcp_usage.output_bytes+EXCLUDED.output_bytes,
 input_tokens_est=codelocal_mcp_usage.input_tokens_est+EXCLUDED.input_tokens_est,
 output_tokens_est=codelocal_mcp_usage.output_tokens_est+EXCLUDED.output_tokens_est,
 last_used_at=GREATEST(codelocal_mcp_usage.last_used_at,EXCLUDED.last_used_at)`, event.UserID, event.Calls, event.InputBytes, event.OutputBytes, event.InputTokensEst, event.OutputTokensEst, event.CreatedAt)
	return err
}

func (s *Store) persistUsageBatch(ctx context.Context, valid []queuedUsageMessage) ([]string, map[usageAggregateKey]MCPUsageEvent, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	ackIDs := make([]string, 0, len(valid))
	dbAggregates := map[string]MCPUsageEvent{}
	windowAggregates := map[usageAggregateKey]MCPUsageEvent{}
	for _, item := range valid {
		tag, err := tx.Exec(ctx, `INSERT INTO codelocal_mcp_usage_batches(id,processed_at) VALUES($1,$2) ON CONFLICT(id) DO NOTHING`, item.batchID, time.Now().UnixMilli())
		if err != nil {
			return nil, nil, err
		}
		ackIDs = append(ackIDs, item.messageID)
		if tag.RowsAffected() > 0 {
			mergeQueuedUsage(item, dbAggregates, windowAggregates)
		}
	}
	for _, event := range dbAggregates {
		if err := persistUsageAggregate(ctx, tx, event); err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return ackIDs, windowAggregates, nil
}

func (s *Store) updateUsageWindows(ctx context.Context, aggregates map[usageAggregateKey]MCPUsageEvent) {
	if len(aggregates) == 0 {
		return
	}
	pipe := s.Redis.Pipeline()
	for _, event := range aggregates {
		hourKey := usageWindowKey(event.UserID, "h", usageBucketStart(event.CreatedAt))
		dayKey := usageWindowKey(event.UserID, "d", usageDayStart(event.CreatedAt))
		for _, key := range []string{hourKey, dayKey} {
			pipe.HIncrBy(ctx, key, "calls", event.Calls)
			pipe.HIncrBy(ctx, key, "input_bytes", int64(event.InputBytes))
			pipe.HIncrBy(ctx, key, "output_bytes", int64(event.OutputBytes))
			pipe.HIncrBy(ctx, key, "input_tokens", int64(event.InputTokensEst))
			pipe.HIncrBy(ctx, key, "output_tokens", int64(event.OutputTokensEst))
		}
		pipe.Expire(ctx, hourKey, 49*time.Hour)
		pipe.Expire(ctx, dayKey, 35*24*time.Hour)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Warn("MCP rolling usage batch update failed", "error", err)
	}
}

func (s *Store) persistMCPUsageMessages(ctx context.Context, messages []redis.XMessage) ([]string, error) {
	valid, ackIDs := parseUsageMessages(messages)
	if len(valid) == 0 {
		return ackIDs, nil
	}
	persistedIDs, windows, err := s.persistUsageBatch(ctx, valid)
	if err != nil {
		return nil, err
	}
	s.updateUsageWindows(ctx, windows)
	return append(ackIDs, persistedIDs...), nil
}

func (s *Store) usageConsumerLeader() bool {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer cancel()
	leader, err := s.refreshUsageConsumerLease(ctx)
	return err == nil && leader
}

func (s *Store) waitForUsageConsumerLease() bool {
	select {
	case <-s.ctx.Done():
		return false
	case <-time.After(time.Second):
		return true
	}
}

func (s *Store) readUsageStreamMessages() []redis.XMessage {
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	messages, _, claimErr := s.Redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: usageStreamKey, Group: usageStreamGroup, Consumer: s.usageConsumerID, MinIdle: 20 * time.Second, Start: "0-0", Count: 128}).Result()
	if claimErr != nil && !errors.Is(claimErr, redis.Nil) && !errors.Is(claimErr, context.Canceled) {
		slog.Warn("MCP usage pending claim failed", "error", claimErr)
	}
	if len(messages) > 0 || (claimErr != nil && !errors.Is(claimErr, redis.Nil)) {
		return messages
	}
	streams, readErr := s.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{Group: usageStreamGroup, Consumer: s.usageConsumerID, Streams: []string{usageStreamKey, ">"}, Count: 128, Block: time.Second}).Result()
	if readErr != nil && !errors.Is(readErr, redis.Nil) && !errors.Is(readErr, context.Canceled) {
		slog.Warn("MCP usage stream read failed", "error", readErr)
	}
	for _, stream := range streams {
		messages = append(messages, stream.Messages...)
	}
	return messages
}

func (s *Store) persistAndAckUsageMessages(messages []redis.XMessage) {
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	ackIDs, err := s.persistMCPUsageMessages(ctx, messages)
	cancel()
	if err != nil {
		slog.Error("MCP usage stream persistence failed", "error", err, "messageCount", len(messages))
		return
	}
	if len(ackIDs) == 0 {
		return
	}
	ackCtx, ackCancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer ackCancel()
	if err := s.Redis.XAck(ackCtx, usageStreamKey, usageStreamGroup, ackIDs...).Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("MCP usage stream ack failed", "error", err, "messageCount", len(ackIDs))
	}
}

func (s *Store) usageStreamConsumer() {
	defer s.wg.Done()
	defer s.releaseUsageConsumerLease()
	for s.ctx.Err() == nil {
		if !s.usageConsumerLeader() {
			if !s.waitForUsageConsumerLease() {
				return
			}
			continue
		}
		messages := s.readUsageStreamMessages()
		if len(messages) > 0 {
			s.persistAndAckUsageMessages(messages)
		}
	}
}

func (s *Store) MCPUsageSummary(ctx context.Context, userID string, since int64) (MCPUsageSummary, error) {
	var out MCPUsageSummary
	if since <= 0 {
		err := s.DB.QueryRow(ctx, `SELECT calls,input_bytes,output_bytes,input_tokens_est,output_tokens_est FROM codelocal_mcp_usage WHERE user_id=$1`, userID).Scan(&out.Calls, &out.InputBytes, &out.OutputBytes, &out.InputTokensEst, &out.OutputTokensEst)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		out.TotalTokensEst = out.InputTokensEst + out.OutputTokensEst
		return out, err
	}

	now := time.Now().UnixMilli()
	useHourly := now-since <= int64(48*time.Hour/time.Millisecond)
	bucket := "d"
	step := int64(24 * time.Hour / time.Millisecond)
	start := usageDayStart(since)
	if useHourly {
		bucket = "h"
		step = int64(time.Hour / time.Millisecond)
		start = usageBucketStart(since)
	}
	keys := make([]string, 0, 32)
	for at := start; at <= now; at += step {
		keys = append(keys, usageWindowKey(userID, bucket, at))
	}
	commands := make([]*redis.MapStringStringCmd, 0, len(keys))
	_, err := s.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, key := range keys {
			commands = append(commands, pipe.HGetAll(ctx, key))
		}
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return out, err
	}
	for _, command := range commands {
		values := command.Val()
		out.Calls += parseUsageInt64(values["calls"])
		out.InputBytes += parseUsageInt64(values["input_bytes"])
		out.OutputBytes += parseUsageInt64(values["output_bytes"])
		out.InputTokensEst += parseUsageInt64(values["input_tokens"])
		out.OutputTokensEst += parseUsageInt64(values["output_tokens"])
	}
	out.TotalTokensEst = out.InputTokensEst + out.OutputTokensEst
	return out, nil
}

type UsageLeaderboardUser struct {
	ID    string
	Email string
}

func (s *Store) ListUsageLeaderboardUsers(ctx context.Context, since int64) ([]UsageLeaderboardUser, error) {
	rows, err := s.DB.Query(ctx, `
SELECT u.id,u.email
FROM codelocal_users u
JOIN codelocal_mcp_usage m ON m.user_id=u.id
WHERE m.last_used_at >= $1
ORDER BY m.last_used_at DESC,u.id ASC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageLeaderboardUser{}
	for rows.Next() {
		var item UsageLeaderboardUser
		if err := rows.Scan(&item.ID, &item.Email); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListAdminUsers(ctx context.Context) ([]AdminUser, error) {
	rows, err := s.DB.Query(ctx, `
SELECT
 u.id,
 u.email,
 u.referral_code,
 COALESCE(u.referred_by_code,''),
 u.created_at,
 (SELECT COUNT(*) FROM codelocal_users child WHERE UPPER(child.referred_by_code)=UPPER(u.referral_code)) AS invite_count,
 COALESCE((SELECT MAX(d.last_seen_at) FROM codelocal_devices d WHERE d.user_id=u.id AND d.revoked_at IS NULL),0) AS last_device_seen_at,
 COALESCE((SELECT m.last_used_at FROM codelocal_mcp_usage m WHERE m.user_id=u.id),0) AS last_mcp_used_at
FROM codelocal_users u
ORDER BY u.created_at ASC,u.email ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var item AdminUser
		if err := rows.Scan(&item.ID, &item.Email, &item.ReferralCode, &item.ReferredByCode, &item.CreatedAt, &item.InviteCount, &item.LastDeviceSeenAt, &item.LastMCPUsedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListInvitedUsers(ctx context.Context, referralCode string) ([]AdminUser, error) {
	referralCode = NormalizeReferralCode(referralCode)
	if !ValidReferralCode(referralCode) {
		return []AdminUser{}, nil
	}
	rows, err := s.DB.Query(ctx, `
SELECT u.id,u.email,u.created_at
FROM codelocal_users u
WHERE UPPER(u.referred_by_code)=UPPER($1)
ORDER BY u.created_at DESC,u.email ASC`, referralCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var item AdminUser
		if err := rows.Scan(&item.ID, &item.Email, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RecentAudit(ctx context.Context, userID string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.DB.Query(ctx, `SELECT id,event,device_id,workspace_id,detail,created_at FROM codelocal_audit_logs WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, event string
		var deviceID, workspaceID *string
		var detail []byte
		var createdAt int64
		if err := rows.Scan(&id, &event, &deviceID, &workspaceID, &detail, &createdAt); err != nil {
			return nil, err
		}
		var decoded map[string]any
		_ = json.Unmarshal(detail, &decoded)
		out = append(out, map[string]any{"id": id, "event": event, "deviceId": deref(deviceID), "workspaceId": deref(workspaceID), "detail": decoded, "createdAt": createdAt})
	}
	return out, rows.Err()
}
