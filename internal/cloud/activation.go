package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type WorkspaceActivation struct {
	WorkspaceID string `json:"workspaceId"`
	RequestedAt int64  `json:"requestedAt"`
	RequestID   string `json:"requestId"`
}

type WorkspaceActivationResult struct {
	WorkspaceID    string `json:"workspaceId"`
	RequestID      string `json:"requestId"`
	OK             bool   `json:"ok"`
	Phase          string `json:"phase,omitempty"`
	Reason         string `json:"reason,omitempty"`
	AcknowledgedAt int64  `json:"acknowledgedAt"`
}

type WorkspaceRevocation = WorkspaceActivation

type ActivationStore struct {
	Redis   *redis.Client
	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
	pubsub  *redis.PubSub
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewActivationStore(ctx context.Context, rdb *redis.Client) *ActivationStore {
	child, cancel := context.WithCancel(ctx)
	s := &ActivationStore{Redis: rdb, waiters: map[string]map[chan struct{}]struct{}{}, ctx: child, cancel: cancel}
	s.pubsub = rdb.Subscribe(child, "codelocal:runtime:signal")
	go s.listen()
	return s
}

func safePart(value string) string { return strings.ReplaceAll(url.QueryEscape(value), "%", "_") }
func (s *ActivationStore) activationKey(user, device string) string {
	return "codelocal:runtime:activation:" + safePart(user) + ":" + safePart(device)
}
func (s *ActivationStore) revocationKey(user, device string) string {
	return "codelocal:runtime:revocation:" + safePart(user) + ":" + safePart(device)
}
func (s *ActivationStore) pendingRevocationKey(user, device, workspace string) string {
	return "codelocal:runtime:revocation-pending:" + safePart(user) + ":" + safePart(device) + ":" + safePart(workspace)
}
func activationResultKey(requestID string) string {
	return "codelocal:runtime:activation-result:" + safePart(requestID)
}
func activationResultChannel(requestID string) string {
	return "codelocal:runtime:activation-result:" + requestID
}
func revocationAckKey(requestID string) string {
	return "codelocal:runtime:revoke-ack-state:" + safePart(requestID)
}
func revocationAckChannel(requestID string) string {
	return "codelocal:runtime:revoke-ack:" + requestID
}
func (s *ActivationStore) presenceKey(user, device string) string {
	return "codelocal:runtime:presence:" + safePart(user) + ":" + safePart(device)
}
func (s *ActivationStore) userPresenceKey(user string) string {
	return "codelocal:user:runtime-active:" + safePart(user)
}
func (s *ActivationStore) authorizedKey(user, device string) string {
	return "codelocal:runtime:authorized:" + safePart(user) + ":" + safePart(device)
}
func waiterKey(user, device string) string { return user + "\x00" + device }

func (s *ActivationStore) listen() {
	channel := s.pubsub.Channel(redis.WithChannelSize(1024))
	for {
		select {
		case <-s.ctx.Done():
			return
		case message, ok := <-channel:
			if !ok {
				return
			}
			var signal struct{ UserID, DeviceID string }
			if json.Unmarshal([]byte(message.Payload), &signal) == nil && signal.UserID != "" && signal.DeviceID != "" {
				s.notify(signal.UserID, signal.DeviceID)
			}
		}
	}
}

func (s *ActivationStore) notify(user, device string) {
	key := waiterKey(user, device)
	s.mu.Lock()
	waiters := s.waiters[key]
	delete(s.waiters, key)
	s.mu.Unlock()
	for ch := range waiters {
		close(ch)
	}
}

func (s *ActivationStore) newWaiter(user, device string) (chan struct{}, func()) {
	key := waiterKey(user, device)
	ch := make(chan struct{})
	s.mu.Lock()
	set := s.waiters[key]
	if set == nil {
		set = map[chan struct{}]struct{}{}
		s.waiters[key] = set
	}
	set[ch] = struct{}{}
	s.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			s.mu.Lock()
			set := s.waiters[key]
			if set != nil {
				delete(set, ch)
				if len(set) == 0 {
					delete(s.waiters, key)
				}
			}
			s.mu.Unlock()
		})
	}
	return ch, cancel
}

func (s *ActivationStore) Heartbeat(ctx context.Context, user, device string, workspaceIDs []string, ttl time.Duration) error {
	if ttl < 20*time.Second {
		ttl = 20 * time.Second
	}
	if len(workspaceIDs) > 500 {
		workspaceIDs = workspaceIDs[:500]
	}
	raw, _ := json.Marshal(workspaceIDs)
	now := time.Now().UnixMilli()
	pipe := s.Redis.Pipeline()
	pipe.Set(ctx, s.presenceKey(user, device), now, ttl)
	pipe.Set(ctx, s.userPresenceKey(user), now, ttl)
	pipe.Set(ctx, s.authorizedKey(user, device), raw, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *ActivationStore) IsOnline(ctx context.Context, user, device string) (bool, error) {
	n, err := s.Redis.Exists(ctx, s.presenceKey(user, device)).Result()
	return n > 0, err
}

func (s *ActivationStore) IsUserOnline(ctx context.Context, user string) (bool, error) {
	n, err := s.Redis.Exists(ctx, s.userPresenceKey(user)).Result()
	return n > 0, err
}

func (s *ActivationStore) UserOnlineMap(ctx context.Context, userIDs []string) (map[string]bool, error) {
	commands := make(map[string]*redis.IntCmd, len(userIDs))
	_, err := s.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, userID := range userIDs {
			if userID == "" {
				continue
			}
			commands[userID] = pipe.Exists(ctx, s.userPresenceKey(userID))
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
func (s *ActivationStore) AuthorizedIDs(ctx context.Context, user, device string) ([]string, error) {
	raw, err := s.Redis.Get(ctx, s.authorizedKey(user, device)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	if json.Unmarshal(raw, &ids) != nil {
		return []string{}, nil
	}
	return ids, nil
}
func (s *ActivationStore) IsAuthorized(ctx context.Context, user, device, workspace string) (bool, error) {
	ids, err := s.AuthorizedIDs(ctx, user, device)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == workspace {
			return true, nil
		}
	}
	return false, nil
}

func (s *ActivationStore) signal(ctx context.Context, user, device string) {
	payload, _ := json.Marshal(map[string]string{"UserID": user, "DeviceID": device})
	_ = s.Redis.Publish(ctx, "codelocal:runtime:signal", payload).Err()
}
func (s *ActivationStore) Request(ctx context.Context, user, device string, a WorkspaceActivation, ttl time.Duration) error {
	raw, _ := json.Marshal(a)
	pipe := s.Redis.Pipeline()
	pipe.RPush(ctx, s.activationKey(user, device), raw)
	pipe.Expire(ctx, s.activationKey(user, device), ttl)
	_, err := pipe.Exec(ctx)
	if err == nil {
		s.signal(ctx, user, device)
	}
	return err
}

func (s *ActivationStore) AcknowledgeActivation(ctx context.Context, result WorkspaceActivationResult) error {
	if result.RequestID == "" || result.WorkspaceID == "" {
		return errors.New("activation result requires requestId and workspaceId")
	}
	if result.AcknowledgedAt == 0 {
		result.AcknowledgedAt = time.Now().UnixMilli()
	}
	raw, _ := json.Marshal(result)
	pipe := s.Redis.Pipeline()
	pipe.Set(ctx, activationResultKey(result.RequestID), raw, 60*time.Second)
	pipe.Publish(ctx, activationResultChannel(result.RequestID), raw)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *ActivationStore) ActivationResult(ctx context.Context, requestID string) (*WorkspaceActivationResult, error) {
	raw, err := s.Redis.Get(ctx, activationResultKey(requestID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result WorkspaceActivationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WaitForActivationResult is race-safe: a durable short-lived result is
// checked before and after the Pub/Sub subscription is established.
func (s *ActivationStore) WaitForActivationResult(ctx context.Context, requestID string) (*WorkspaceActivationResult, error) {
	if result, err := s.ActivationResult(ctx, requestID); err != nil || result != nil {
		return result, err
	}
	pubsub := s.Redis.Subscribe(ctx, activationResultChannel(requestID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, err
	}
	if result, err := s.ActivationResult(ctx, requestID); err != nil || result != nil {
		return result, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-pubsub.Channel():
		return s.ActivationResult(ctx, requestID)
	}
}

func (s *ActivationStore) LastHeartbeatAt(ctx context.Context, user, device string) (int64, error) {
	value, err := s.Redis.Get(ctx, s.presenceKey(user, device)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return value, err
}

func (s *ActivationStore) RequestRevocation(ctx context.Context, user, device string, a WorkspaceRevocation, ttl time.Duration) error {
	raw, _ := json.Marshal(a)
	pipe := s.Redis.Pipeline()
	pipe.RPush(ctx, s.revocationKey(user, device), raw)
	pipe.Expire(ctx, s.revocationKey(user, device), ttl)
	pipe.Set(ctx, s.pendingRevocationKey(user, device, a.WorkspaceID), a.RequestID, ttl)
	_, err := pipe.Exec(ctx)
	if err == nil {
		s.signal(ctx, user, device)
	}
	return err
}

// AcknowledgeRevocation completes both modern explicit acknowledgements and
// legacy revoke-by-sync flows. If requestID is empty, the currently pending
// request for this workspace is resolved from Redis.
func (s *ActivationStore) AcknowledgeRevocation(ctx context.Context, user, device, workspace, requestID string) (bool, error) {
	pendingKey := s.pendingRevocationKey(user, device, workspace)
	expected, err := s.Redis.Get(ctx, pendingKey).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if requestID == "" {
		requestID = expected
	}
	if requestID == "" || requestID != expected {
		return false, nil
	}
	payload, _ := json.Marshal(map[string]any{"deviceId": device, "workspaceId": workspace})
	pipe := s.Redis.Pipeline()
	pipe.Set(ctx, revocationAckKey(requestID), "1", 30*time.Second)
	pipe.Del(ctx, pendingKey)
	pipe.Publish(ctx, revocationAckChannel(requestID), payload)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// WaitForRevocationAck is race-safe: the durable short-lived ack key is
// checked both before and after subscribing, so an acknowledgement cannot be
// lost between RequestRevocation and Pub/Sub subscription setup.
func (s *ActivationStore) WaitForRevocationAck(ctx context.Context, requestID string, timeout time.Duration) error {
	if ok, err := s.Redis.Exists(ctx, revocationAckKey(requestID)).Result(); err != nil {
		return err
	} else if ok > 0 {
		return nil
	}
	pubsub := s.Redis.Subscribe(ctx, revocationAckChannel(requestID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	if ok, err := s.Redis.Exists(ctx, revocationAckKey(requestID)).Result(); err != nil {
		return err
	} else if ok > 0 {
		return nil
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("workspace revocation timed out")
	case <-pubsub.Channel():
		return nil
	}
}

func pop[T any](ctx context.Context, rdb *redis.Client, key string) (*T, error) {
	raw, err := rdb.LPop(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out T
	if json.Unmarshal(raw, &out) != nil {
		return nil, nil
	}
	return &out, nil
}

func (s *ActivationStore) WaitForNext(ctx context.Context, user, device string, timeout time.Duration) (*WorkspaceActivation, *WorkspaceRevocation, error) {
	if timeout < 0 {
		timeout = 0
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		var waiter chan struct{}
		cancel := func() {}
		remaining := time.Until(deadline)
		if timeout > 0 && remaining > 0 {
			waiter, cancel = s.newWaiter(user, device)
		}
		rev, err := pop[WorkspaceRevocation](ctx, s.Redis, s.revocationKey(user, device))
		if err != nil {
			cancel()
			return nil, nil, err
		}
		if rev != nil {
			cancel()
			return nil, rev, nil
		}
		act, err := pop[WorkspaceActivation](ctx, s.Redis, s.activationKey(user, device))
		if err != nil {
			cancel()
			return nil, nil, err
		}
		if act != nil {
			cancel()
			return act, nil, nil
		}
		if waiter == nil {
			return nil, nil, nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			cancel()
			return nil, nil, ctx.Err()
		case <-timer.C:
			cancel()
			return nil, nil, nil
		case <-waiter:
			if !timer.Stop() {
				<-timer.C
			}
			cancel()
		}
	}
}

func (s *ActivationStore) ClearPresence(ctx context.Context, user, device string) error {
	return s.Redis.Del(ctx, s.presenceKey(user, device), s.authorizedKey(user, device), s.activationKey(user, device), s.revocationKey(user, device)).Err()
}
func (s *ActivationStore) Close() error {
	s.cancel()
	s.mu.Lock()
	for _, set := range s.waiters {
		for ch := range set {
			close(ch)
		}
	}
	s.waiters = map[string]map[chan struct{}]struct{}{}
	s.mu.Unlock()
	return s.pubsub.Close()
}
