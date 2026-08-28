package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type RoutedCall struct {
	Kind           string         `json:"kind"`
	SourceInstance string         `json:"sourceInstance"`
	RequestID      string         `json:"requestId"`
	UserID         string         `json:"userId"`
	ClientKey      string         `json:"clientKey"`
	SessionID      string         `json:"sessionId,omitempty"`
	Tool           string         `json:"tool"`
	Args           map[string]any `json:"args,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
	Deadline       int64          `json:"deadline,omitempty"`
	Reason         string         `json:"reason,omitempty"`
}

type RoutedResult struct {
	RequestID string `json:"requestId"`
	OK        bool   `json:"ok"`
	Result    any    `json:"result,omitempty"`
	Metadata  any    `json:"metadata,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Handler func(context.Context, RoutedCall) RoutedResult
type CredentialDisconnectHandler func(userID, credentialID string)

const credentialDisconnectChannel = "codelocal:gateway:credential-disconnect"

type Coordinator struct {
	Redis                  *redis.Client
	InstanceID             string
	ctx                    context.Context
	cancel                 context.CancelFunc
	handler                Handler
	onCredentialDisconnect CredentialDisconnectHandler
	pubsub                 *redis.PubSub
	mu                     sync.Mutex
	pending                map[string]chan RoutedResult
	active                 map[string]context.CancelFunc
	ownerWaiters           map[string]map[chan struct{}]struct{}
}

func NewCoordinator(ctx context.Context, rdb *redis.Client, instanceID string, handler Handler, onCredentialDisconnect CredentialDisconnectHandler) *Coordinator {
	child, cancel := context.WithCancel(ctx)
	c := &Coordinator{
		Redis:                  rdb,
		InstanceID:             instanceID,
		ctx:                    child,
		cancel:                 cancel,
		handler:                handler,
		onCredentialDisconnect: onCredentialDisconnect,
		pending:                map[string]chan RoutedResult{},
		active:                 map[string]context.CancelFunc{},
		ownerWaiters:           map[string]map[chan struct{}]struct{}{},
	}
	c.pubsub = rdb.Subscribe(child, c.requestChannel(), c.responseChannel(), c.cancelChannel(), "codelocal:gateway:owner-signal", credentialDisconnectChannel)
	go c.listen()
	return c
}

func safe(value string) string { return url.QueryEscape(value) }
func (c *Coordinator) requestChannel() string {
	return "codelocal:gateway:req:" + safe(c.InstanceID)
}
func (c *Coordinator) responseChannel() string {
	return "codelocal:gateway:resp:" + safe(c.InstanceID)
}
func (c *Coordinator) cancelChannel() string {
	return "codelocal:gateway:cancel:" + safe(c.InstanceID)
}
func ownerKey(clientKey string) string { return "codelocal:gateway:owner:" + safe(clientKey) }

func (c *Coordinator) listen() {
	ch := c.pubsub.Channel(redis.WithChannelSize(4096))
	for {
		select {
		case <-c.ctx.Done():
			return
		case message, ok := <-ch:
			if !ok {
				return
			}
			switch message.Channel {
			case c.requestChannel():
				var call RoutedCall
				if json.Unmarshal([]byte(message.Payload), &call) == nil && call.RequestID != "" {
					go c.handleRemote(call)
				}
			case c.responseChannel():
				var result RoutedResult
				if json.Unmarshal([]byte(message.Payload), &result) == nil && result.RequestID != "" {
					c.mu.Lock()
					waiter := c.pending[result.RequestID]
					if waiter != nil {
						delete(c.pending, result.RequestID)
					}
					c.mu.Unlock()
					if waiter != nil {
						select {
						case waiter <- result:
						default:
						}
					}
				}
			case c.cancelChannel():
				var call RoutedCall
				if json.Unmarshal([]byte(message.Payload), &call) == nil && call.RequestID != "" {
					c.mu.Lock()
					cancel := c.active[call.RequestID]
					c.mu.Unlock()
					if cancel != nil {
						cancel()
					}
				}
			case "codelocal:gateway:owner-signal":
				var signal struct {
					ClientKey string `json:"clientKey"`
				}
				if json.Unmarshal([]byte(message.Payload), &signal) == nil && signal.ClientKey != "" {
					c.notifyOwner(signal.ClientKey)
				}
			case credentialDisconnectChannel:
				var event struct {
					UserID       string `json:"userId"`
					CredentialID string `json:"credentialId"`
				}
				if json.Unmarshal([]byte(message.Payload), &event) == nil && event.UserID != "" && event.CredentialID != "" && c.onCredentialDisconnect != nil {
					c.onCredentialDisconnect(event.UserID, event.CredentialID)
				}
			}
		}
	}
}

func (c *Coordinator) handleRemote(call RoutedCall) {
	ctx := c.ctx
	var cancel context.CancelFunc
	if call.Deadline > 0 {
		ctx, cancel = context.WithDeadline(ctx, time.UnixMilli(call.Deadline))
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	c.mu.Lock()
	c.active[call.RequestID] = cancel
	c.mu.Unlock()
	defer func() {
		cancel()
		c.mu.Lock()
		delete(c.active, call.RequestID)
		c.mu.Unlock()
	}()

	result := c.handler(ctx, call)
	payload, _ := json.Marshal(result)
	channel := "codelocal:gateway:resp:" + safe(call.SourceInstance)
	if err := c.Redis.Publish(c.ctx, channel, payload).Err(); err != nil {
		slog.Error("gateway response publish failed", "error", err, "requestId", call.RequestID)
	}
}

func (c *Coordinator) BroadcastCredentialDisconnect(ctx context.Context, userID, credentialID string) error {
	if userID == "" || credentialID == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"userId": userID, "credentialId": credentialID})
	return c.Redis.Publish(ctx, credentialDisconnectChannel, payload).Err()
}

func (c *Coordinator) notifyOwner(clientKey string) {
	c.mu.Lock()
	waiters := c.ownerWaiters[clientKey]
	delete(c.ownerWaiters, clientKey)
	c.mu.Unlock()
	for waiter := range waiters {
		close(waiter)
	}
}

func (c *Coordinator) ownerWaiter(clientKey string) (chan struct{}, func()) {
	waiter := make(chan struct{})
	c.mu.Lock()
	set := c.ownerWaiters[clientKey]
	if set == nil {
		set = map[chan struct{}]struct{}{}
		c.ownerWaiters[clientKey] = set
	}
	set[waiter] = struct{}{}
	c.mu.Unlock()

	var once sync.Once
	return waiter, func() {
		once.Do(func() {
			c.mu.Lock()
			set := c.ownerWaiters[clientKey]
			if set != nil {
				delete(set, waiter)
				if len(set) == 0 {
					delete(c.ownerWaiters, clientKey)
				}
			}
			c.mu.Unlock()
		})
	}
}

func (c *Coordinator) ClaimOwner(ctx context.Context, clientKey string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 90 * time.Second
	}
	if err := c.Redis.Set(ctx, ownerKey(clientKey), c.InstanceID, ttl).Err(); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"clientKey": clientKey, "owner": c.InstanceID})
	_ = c.Redis.Publish(ctx, "codelocal:gateway:owner-signal", payload).Err()
	return nil
}

func (c *Coordinator) RefreshOwner(ctx context.Context, clientKey string, ttl time.Duration) error {
	current, err := c.Redis.Get(ctx, ownerKey(clientKey)).Result()
	if errors.Is(err, redis.Nil) {
		return c.ClaimOwner(ctx, clientKey, ttl)
	}
	if err != nil {
		return err
	}
	if current != c.InstanceID {
		return fmt.Errorf("workspace connection is owned by %s", current)
	}
	return c.Redis.Expire(ctx, ownerKey(clientKey), ttl).Err()
}

func (c *Coordinator) ReleaseOwner(ctx context.Context, clientKey string) {
	script := `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`
	_ = c.Redis.Eval(ctx, script, []string{ownerKey(clientKey)}, c.InstanceID).Err()
}

func (c *Coordinator) Owner(ctx context.Context, clientKey string) (string, error) {
	owner, err := c.Redis.Get(ctx, ownerKey(clientKey)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return owner, err
}

func (c *Coordinator) WaitOwner(ctx context.Context, clientKey string) (string, error) {
	for {
		owner, err := c.Owner(ctx, clientKey)
		if err != nil {
			return "", err
		}
		if owner != "" {
			return owner, nil
		}

		waiter, cancel := c.ownerWaiter(clientKey)
		owner, err = c.Owner(ctx, clientKey)
		if err != nil {
			cancel()
			return "", err
		}
		if owner != "" {
			cancel()
			return owner, nil
		}

		select {
		case <-ctx.Done():
			cancel()
			return "", ctx.Err()
		case <-waiter:
			cancel()
		}
	}
}

func (c *Coordinator) Call(ctx context.Context, call RoutedCall) (RoutedResult, error) {
	resolvedKey, err := c.ResolveRuntimeAlias(ctx, call.ClientKey)
	if err != nil {
		return RoutedResult{}, err
	}
	call.ClientKey = resolvedKey
	owner, err := c.Owner(ctx, call.ClientKey)
	if err != nil {
		return RoutedResult{}, err
	}
	if owner == "" {
		return RoutedResult{}, errors.New("workspace is offline")
	}
	call.SourceInstance = c.InstanceID
	if owner == c.InstanceID {
		return c.handler(ctx, call), nil
	}

	waiter := make(chan RoutedResult, 1)
	c.mu.Lock()
	c.pending[call.RequestID] = waiter
	c.mu.Unlock()

	payload, _ := json.Marshal(call)
	if err := c.Redis.Publish(ctx, "codelocal:gateway:req:"+safe(owner), payload).Err(); err != nil {
		c.mu.Lock()
		delete(c.pending, call.RequestID)
		c.mu.Unlock()
		return RoutedResult{}, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, call.RequestID)
		c.mu.Unlock()
		cancelPayload, _ := json.Marshal(RoutedCall{Kind: "cancel", RequestID: call.RequestID, Reason: ctx.Err().Error()})
		_ = c.Redis.Publish(context.Background(), "codelocal:gateway:cancel:"+safe(owner), cancelPayload).Err()
		return RoutedResult{}, ctx.Err()
	case result, ok := <-waiter:
		if !ok {
			return RoutedResult{}, errors.New("gateway coordinator closed")
		}
		return result, nil
	}
}

func (c *Coordinator) Close() error {
	c.cancel()
	c.mu.Lock()
	for _, cancel := range c.active {
		cancel()
	}
	c.active = map[string]context.CancelFunc{}
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	for key, waiters := range c.ownerWaiters {
		for waiter := range waiters {
			close(waiter)
		}
		delete(c.ownerWaiters, key)
	}
	c.mu.Unlock()
	return c.pubsub.Close()
}
