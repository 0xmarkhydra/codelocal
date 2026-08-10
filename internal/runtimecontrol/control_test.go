package runtimecontrol

import (
	"context"
	"testing"
	"time"
)

func TestSingletonLeaseUsesLiveControlIdentity(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire("test", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Acquired {
		t.Fatal("first lease should be acquired")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, dir, first.Record.InstanceID, func(ctx context.Context, cmd Command) (any, error) {
		return map[string]any{"phase": "online"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	defer first.Release()

	second, err := Acquire("test", dir)
	if err != nil {
		t.Fatal(err)
	}
	if second.Acquired {
		t.Fatal("second lease must reuse the running runtime")
	}
	if second.Record.InstanceID != first.Record.InstanceID {
		t.Fatal("runtime instance identity changed")
	}

	summary := Summary(context.Background(), dir)
	if summary["running"] != true || summary["responsive"] != true {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	_ = server.Close()
	_ = first.Release()
	time.Sleep(20 * time.Millisecond)
	third, err := Acquire("test2", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !third.Acquired {
		t.Fatal("lease should be acquirable after shutdown")
	}
	_ = third.Release()
}
