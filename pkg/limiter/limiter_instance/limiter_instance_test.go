package limiter_instance

import (
	"context"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_instance_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_manager_api"
	"testing"
	"time"
)

func TestNew_instance_can_be_closed(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		parentChan,
	)

	instance <- &limiter_instance_api.Kill{}
	closed := false

	select {
	case msg := <-parentChan:
		switch msg.(type) {
		case *limiter_manager_api.InstanceDiedNotification:
			closed = true
		default:
			t.Fatalf("Unexpected message: %v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("expected instance to close")
	}

	if !closed {
		t.Fatalf("expected instance to close")
	}
}

func TestNew_instance_can_be_queried_for_debug_snapshot(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		parentChan,
	)
	defer func() { instance <- &limiter_instance_api.Kill{} }()

	snapshot := requestDebugSnapshot(t, instance, "key")

	if diff := cmp.Diff(snapshot, &limiter_api.InstanceDebugSnapshot{
		Key:                   "key",
		NumApprovedThisWindow: 0,
		NumWaiting:            0,
		Config: limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		Found: true,
	}); diff != "" {
		t.Fatalf("unexpected snapshot (-want +got):\n%s", diff)
	}
}

func TestNew_instance_can_display_send_status(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		parentChan,
	)
	defer func() { instance <- &limiter_instance_api.Kill{} }()

	sendResult := requestPermission(t, instance, "key", false)
	if sendResult.RespCode != limiter_api.Approved {
		t.Fatalf("expected approved")
	}

	debugSnapshot := requestDebugSnapshot(t, instance, "key")
	if diff := cmp.Diff(debugSnapshot, &limiter_api.InstanceDebugSnapshot{
		Key:                   "key",
		NumApprovedThisWindow: 1,
		NumWaiting:            0,
		Config: limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		Found: true,
	}); diff != "" {
		t.Fatalf("unexpected snapshot (-want +got):\n%s", diff)
	}

}

func TestNew_can_display_updated_config(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		parentChan,
	)
	defer func() { instance <- &limiter_instance_api.Kill{} }()

	instance <- &limiter_instance_api.ConfigUpdateNotification{
		Config: &limiter_api.Config{
			WindowMillis:         10_001,
			MaxRequestsPerWindow: 11,
			MaxRequestsInQueue:   12,
		},
	}

	debugSnapshot := requestDebugSnapshot(t, instance, "key")
	if diff := cmp.Diff(debugSnapshot, &limiter_api.InstanceDebugSnapshot{
		Key:                   "key",
		NumApprovedThisWindow: 0,
		NumWaiting:            0,
		Config: limiter_api.Config{
			WindowMillis:         10_001,
			MaxRequestsPerWindow: 11,
			MaxRequestsInQueue:   12,
		},
		Found: true,
	}); diff != "" {
		t.Fatalf("unexpected snapshot (-want +got):\n%s", diff)
	}

}

func TestNew_can_config_values_only_update_on_nonzero(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		parentChan,
	)
	defer func() { instance <- &limiter_instance_api.Kill{} }()

	instance <- &limiter_instance_api.ConfigUpdateNotification{
		Config: &limiter_api.Config{
			WindowMillis:         0,
			MaxRequestsPerWindow: 0,
			MaxRequestsInQueue:   0,
		},
	}

	debugSnapshot := requestDebugSnapshot(t, instance, "key")

	if diff := cmp.Diff(debugSnapshot, &limiter_api.InstanceDebugSnapshot{
		Key:                   "key",
		NumApprovedThisWindow: 0,
		NumWaiting:            0,
		Config: limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		Found: true,
	}); diff != "" {
		t.Fatalf("unexpected snapshot (-want +got):\n%s", diff)
	}

}

func TestNew_approves_max_request_but_not_more(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		parentChan,
	)
	defer func() { instance <- &limiter_instance_api.Kill{} }()

	for i := 0; i < 10; i++ {
		sendResult := requestPermission(t, instance, "key", false)
		if sendResult.RespCode != limiter_api.Approved {
			t.Fatalf("expected approved")
		}
	}

	sendResult := requestPermission(t, instance, "key", false)
	if sendResult.RespCode != limiter_api.Denied {
		t.Fatalf("expected denied")
	}
}

func TestNew_approves_can_queue_a_lot(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         10_000,
			MaxRequestsPerWindow: 1,
			MaxRequestsInQueue:   1000,
		},
		parentChan,
	)
	defer func() { instance <- &limiter_instance_api.Kill{} }()

	for i := 0; i < 1001; i++ {
		go func() {
			instance <- &limiter_api.PermissionRequest{
				ReqID:              uuid.New().String(),
				Key:                "key",
				RespChan:           make(chan *limiter_api.PermissionResponse, 1),
				Ctx:                context.Background(),
				CanWait:            true,
				MaxRequests:        limiter_api.NoChange,
				MaxRequestsInQueue: limiter_api.NoChange,
			}
		}()
	}

	// wait until 1000 are queued

	t0 := time.Now()

	for time.Since(t0) < 5*time.Second {
		debugSnapshot := requestDebugSnapshot(t, instance, "key")
		if debugSnapshot.NumWaiting == 1000 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	debugSnapshot := requestDebugSnapshot(t, instance, "key")
	if debugSnapshot.NumWaiting != 1000 {
		t.Fatalf("expected 1000 in queue")
	}
}

func TestNew_approves_max_request_plus_queue_but_not_more(t *testing.T) {

	parentChan := make(chan limiter_manager_api.Request, 10)

	instance := New(
		"key",
		&limiter_api.Config{
			WindowMillis:         1_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		parentChan,
	)
	defer func() { instance <- &limiter_instance_api.Kill{} }()

	t0 := time.Now()

	for i := 0; i < 10; i++ {
		sendResult := requestPermission(t, instance, "key", true)
		if sendResult.RespCode != limiter_api.Approved {
			t.Fatalf("expected approved")
		}
	}

	// state should now be 10 approved, 0 in queue
	debugSnapshot := requestDebugSnapshot(t, instance, "key")
	if diff := cmp.Diff(debugSnapshot, &limiter_api.InstanceDebugSnapshot{
		Key:                   "key",
		NumApprovedThisWindow: 10,
		NumWaiting:            0,
		Config: limiter_api.Config{
			WindowMillis:         1_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		Found: true,
	}); diff != "" {
		t.Fatalf("unexpected snapshot (-want +got):\n%s", diff)
	}

	for i := 0; i < 10; i++ {
		sendResult := requestPermission(t, instance, "key", true)
		if sendResult.RespCode != limiter_api.Approved {
			t.Fatalf("expected approved")
		}
	}

	if time.Since(t0) < 500*time.Millisecond {
		t.Fatalf("expected to wait at least a bit")
	}

	if time.Since(t0) > 2_000*time.Millisecond {
		t.Fatalf("expected to not wait too long")
	}

	sendResult := requestPermission(t, instance, "key", false)
	if sendResult.RespCode != limiter_api.Denied {
		t.Fatalf("expected denied")
	}

	// state should now be 10 approved, 0 in queue
	debugSnapshot = requestDebugSnapshot(t, instance, "key")
	if diff := cmp.Diff(debugSnapshot, &limiter_api.InstanceDebugSnapshot{
		Key:                   "key",
		NumApprovedThisWindow: 10,
		NumDeniedThisWindow:   1,
		NumWaiting:            0,
		Config: limiter_api.Config{
			WindowMillis:         1_000,
			MaxRequestsPerWindow: 10,
			MaxRequestsInQueue:   10,
		},
		Found: true,
	}); diff != "" {
		t.Fatalf("unexpected snapshot (-want +got):\n%s", diff)
	}
}

func awaitSnapshot(t *testing.T, debugSnapshotChan chan *limiter_api.InstanceDebugSnapshot) *limiter_api.InstanceDebugSnapshot {
	for {
		select {
		case snapshot := <-debugSnapshotChan:
			return snapshot
		case <-time.After(5 * time.Second):
			t.Fatalf("expected instance to close")
		}
	}
}

func awaitPermissionResponse(t *testing.T, respChan chan *limiter_api.PermissionResponse) *limiter_api.PermissionResponse {
	for {
		select {
		case resp := <-respChan:
			return resp
		case <-time.After(5 * time.Second):
			t.Fatalf("expected instance to close")
		}
	}
}

func requestDebugSnapshot(t *testing.T, instance chan<- limiter_instance_api.Request, key string) *limiter_api.InstanceDebugSnapshot {
	debugSnapshotChan := make(chan *limiter_api.InstanceDebugSnapshot, 10)
	instance <- &limiter_api.DebugSnapshotRequest{Key: key, RespChan: debugSnapshotChan}
	return awaitSnapshot(t, debugSnapshotChan)
}

func requestPermission(t *testing.T, instance chan<- limiter_instance_api.Request, key string, canWait bool) *limiter_api.PermissionResponse {
	respChan := make(chan *limiter_api.PermissionResponse, 10)
	instance <- &limiter_api.PermissionRequest{
		ReqID:              uuid.New().String(),
		Key:                key,
		RespChan:           respChan,
		Ctx:                context.Background(),
		CanWait:            canWait,
		MaxRequests:        limiter_api.NoChange,
		MaxRequestsInQueue: limiter_api.NoChange,
	}
	return awaitPermissionResponse(t, respChan)
}
