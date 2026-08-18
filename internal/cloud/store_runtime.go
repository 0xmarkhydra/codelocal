package cloud

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	StoreMigrationStartup  = "startup"
	StoreMigrationExternal = "external"
	StoreMigrationOnly     = "only"
)

type StoreOptions struct {
	RunMigrations bool
	StartWorkers  bool
}

func StoreMigrationMode() (string, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_MIGRATION_MODE")))
	if mode == "" {
		return StoreMigrationStartup, nil
	}
	switch mode {
	case StoreMigrationStartup, StoreMigrationExternal, StoreMigrationOnly:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid CODELOCAL_MIGRATION_MODE %q", mode)
	}
}

func New(ctx context.Context) (*Store, error) {
	mode, err := StoreMigrationMode()
	if err != nil {
		return nil, err
	}
	return NewWithOptions(ctx, StoreOptions{RunMigrations: mode != StoreMigrationExternal, StartWorkers: true})
}

func NewWithOptions(ctx context.Context, options StoreOptions) (*Store, error) {
	databaseURL, redisURL, err := storeURLs()
	if err != nil {
		return nil, err
	}
	db, err := openStoreDB(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	rdb, err := openStoreRedis(ctx, redisURL)
	if err != nil {
		db.Close()
		return nil, err
	}
	store := assembleStore(ctx, db, rdb)
	if err := initializeStore(ctx, store, options.RunMigrations); err != nil {
		store.Close()
		return nil, err
	}
	if options.StartWorkers {
		startStoreWorkers(store)
	}
	return store, nil
}

func MigrateOnly(ctx context.Context) error {
	store, err := NewWithOptions(ctx, StoreOptions{RunMigrations: true, StartWorkers: false})
	if err != nil {
		return err
	}
	store.Close()
	return nil
}

func storeURLs() (string, string, error) {
	databaseURL, redisURL := os.Getenv("DATABASE_URL"), os.Getenv("REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		return "", "", errors.New("CodeLocal Cloud requires DATABASE_URL and REDIS_URL")
	}
	return databaseURL, redisURL, nil
}

func openStoreDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if raw := os.Getenv("CODELOCAL_DB_POOL_SIZE"); raw != "" {
		if value, parseErr := strconv.Atoi(raw); parseErr == nil && value > 0 {
			config.MaxConns = int32(value)
		}
	}
	config.MinConns = min32(2, config.MaxConns)
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 45 * time.Minute
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func openStoreRedis(ctx context.Context, redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	options.PoolSize = envInt("CODELOCAL_REDIS_POOL_SIZE", 32)
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func assembleStore(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client) *Store {
	storeCtx, cancel := context.WithCancel(ctx)
	embeddingProvider, embeddingErr := canonicalEmbeddingProviderFromEnv()
	store := &Store{
		DB: db, Redis: rdb, ctx: storeCtx, cancel: cancel,
		usageQ: make(chan MCPUsageEvent, envInt("CODELOCAL_USAGE_LOCAL_QUEUE_SIZE", 8192)), usageConsumerID: RandomHex(12),
		outboxWorkerID: RandomHex(12), outboxWake: make(chan struct{}, 1), canonicalEmbeddingProvider: embeddingProvider,
	}
	if embeddingErr != nil {
		store.canonicalEmbeddingError = embeddingErr.Error()
		slog.Warn("canonical embedding provider unavailable", "error", embeddingErr)
	}
	return store
}

func initializeStore(ctx context.Context, store *Store, runMigrations bool) error {
	if runMigrations {
		if err := store.Migrate(ctx); err != nil {
			return err
		}
		if err := store.ensureCanonicalEmbeddingVectorSchema(ctx); err != nil {
			store.canonicalEmbeddingProvider = nil
			store.canonicalEmbeddingError = "canonical embedding vector schema unavailable: " + err.Error()
			slog.Warn("canonical embedding vector schema unavailable; derived semantic index disabled", "error", err)
		}
	}
	return store.ensureUsageStreamGroup(ctx)
}

func startStoreWorkers(store *Store) {
	store.wg.Add(5)
	go store.usageStreamProducer()
	go store.usageStreamConsumer()
	go store.retentionWorker()
	go store.durableOutboxWorker()
	go store.knowledgeHealthWorker()
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}
