package taskexecution

import "testing"

func TestRuntimeLifecycleFullPath(t *testing.T) {
	l := RuntimeLifecycle{State: LifecycleCold}
	var err error
	if l, err = l.Provision(); err != nil {
		t.Fatal(err)
	}
	if l, err = l.MarkReady(); err != nil {
		t.Fatal(err)
	}
	if l, err = l.Activate(); err != nil {
		t.Fatal(err)
	}
	if l, err = l.MarkIdle(); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Sleep(); err == nil {
		t.Fatal("sleep without checkpoint accepted")
	}
	if l, err = l.Checkpoint("digest-1"); err != nil {
		t.Fatal(err)
	}
	if l, err = l.Sleep(); err != nil {
		t.Fatal(err)
	}
	if l.CheckpointDigest != "digest-1" {
		t.Fatal("sleep dropped checkpoint reference")
	}
	if l, err = l.Wake(); err != nil {
		t.Fatal(err)
	}
	if l.State != LifecycleReady || l.CheckpointDigest != "" {
		t.Fatalf("wake state mismatch: %+v", l)
	}
	if l, err = l.Terminate(); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Activate(); err == nil {
		t.Fatal("transition out of terminated accepted")
	}
}

func TestRuntimeLifecycleRejectsJumps(t *testing.T) {
	l := RuntimeLifecycle{State: LifecycleCold}
	if _, err := l.MarkReady(); err == nil {
		t.Fatal("cold to ready accepted")
	}
	if _, err := l.Terminate(); err != nil {
		t.Fatal(err)
	}
}
