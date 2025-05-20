package limiter_manager

import (
	"context"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/kivra/gocc/pkg/config"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/kivra/gocc/pkg/logging"
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterManager_AskPermission_cantWait(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         10_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, 1)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	result, _ := mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)

	if result != limiter_api.Approved {
		t.Errorf("expected Approved, got %v", result)
	}

	if time.Since(t0) > 500*time.Millisecond {
		t.Errorf("expected to return immediately, but took %v", time.Since(t0))
	}
}

func TestLimiterManager_AskPermission_cantWait_hashing(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         10_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	result, _ := mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)

	if result != limiter_api.Approved {
		t.Errorf("expected Approved, got %v", result)
	}

	if time.Since(t0) > 500*time.Millisecond {
		t.Errorf("expected to return immediately, but took %v", time.Since(t0))
	}
}

func TestLimiterManager_AskPermission_getAllDebugSnapshot(t *testing.T) {

	logging.ConfigureDefaultLogger("text", "WARN", false)
	defer logging.ConfigureDefaultLogger("text", "INFO", false)

	nKeys := 1000

	globalCfg := &limiter_api.Config{
		WindowMillis:         10_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()

	for i := 0; i < nKeys; i++ {
		result, _ := mgr.AskPermission(ctx, "key"+strconv.Itoa(i), false, limiter_api.NoChange, limiter_api.NoChange)

		if result != limiter_api.Approved {
			t.Errorf("expected Approved, got %v", result)
		}
	}

	snapshotsAll := mgr.GetDebugSnapshotsAll()

	if snapshotsAll == nil {
		t.Fatalf("expected snapshotsAll, got nil")
	}

	if len(snapshotsAll.Instances) != nKeys {
		t.Fatalf("expected %d keys, got %d", nKeys, len(snapshotsAll.Instances))
	}

}
func TestLimiterManager_AskPermission_getDebugSnapshot(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         10_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()

	result, _ := mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)

	if result != limiter_api.Approved {
		t.Errorf("expected Approved, got %v", result)
	}

	snapshot := mgr.GetDebugSnapshot("key")
	if snapshot == nil {
		t.Fatalf("expected snapshot, got nil")
	}

	if diff := cmp.Diff(snapshot, &limiter_api.InstanceDebugSnapshot{
		Key: "key",
		Config: limiter_api.Config{
			WindowMillis:         10000,
			MaxRequestsPerWindow: 1,
			MaxRequestsInQueue:   100,
		},
		NumApprovedThisWindow: 1,
		NumWaiting:            0,
		Found:                 true,
	}); diff != "" {
		t.Errorf("unexpected snapshot (-want +got):\n%s", diff)
	}
}

func TestLimiterManager_AskPermission_canWait(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         10_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	result, _ := mgr.AskPermission(ctx, "key", true, limiter_api.NoChange, limiter_api.NoChange)

	if result != limiter_api.Approved {
		t.Errorf("expected Approved, got %v", result)
	}

	if time.Since(t0) > 500*time.Millisecond {
		t.Errorf("expected to return immediately, but took %v", time.Since(t0))
	}

	mgr.Close()
}

func TestLimiterManager_request_timesOut(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         10_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	result, _ := mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Approved {
		t.Errorf("expected Approved, got %v", result)
	}

	result, _ = mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Denied {
		t.Errorf("expected Denied, got %v", result)
	}

	if time.Since(t0) > 500*time.Millisecond {
		t.Errorf("expected to return immediately, but took %v", time.Since(t0))
	}
}

func TestLimiterManager_request_waitsForItsTurn_canWait(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         1_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	result, _ := mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Approved {
		t.Errorf("expected Approved, got %v", result)
	}

	result, _ = mgr.AskPermission(ctx, "key", true, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Approved {
		t.Errorf("expected Denied, got %v", result)
	}

	if time.Since(t0) < 500*time.Millisecond {
		t.Errorf("expected to return after sometime, but finished too quickly %v", time.Since(t0))
	}
}

func TestLimiterManager_request_can_override_request_per_window(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         1_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	result, _ := mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Approved {
		t.Errorf("expected Approved, got %v", result)
	}

	result, _ = mgr.AskPermission(ctx, "key", true, 2, limiter_api.NoChange)
	if result != limiter_api.Approved {
		t.Errorf("expected Denied, got %v", result)
	}

	if time.Since(t0) > 500*time.Millisecond {
		t.Errorf("expected to return immediately, but took %v", time.Since(t0))
	}
}

func TestLimiterManager_request_can_override_max_queued_request_per_window(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         50,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   1,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, 10)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	errs := make(chan error, 10)
	lop.ForEach(lo.Range(10), func(_ int, _ int) {
		result, _ := mgr.AskPermission(ctx, "key", true, limiter_api.NoChange, limiter_api.NoChange)
		if result != limiter_api.Approved {
			errs <- fmt.Errorf("denied %v", result)
		}
	})
	// must get at least 3 denials
	if len(errs) < 3 {
		t.Errorf("expected at least 3 denials, got %d", len(errs))
	}

	lop.ForEach(lo.Range(10), func(_ int, _ int) {
		result, _ := mgr.AskPermission(ctx, "key", true, limiter_api.NoChange, DefaultSharding)
		if result != limiter_api.Approved {
			t.Fatalf("expected Approved, got %v", result)
		}
	})

	if time.Since(t0) > 5*time.Second {
		t.Errorf("expected to return immediately, but took %v", time.Since(t0))
	}
}

func TestLimiterManager_independent_keys_dont_interfere(t *testing.T) {
	globalCfg := &limiter_api.Config{
		WindowMillis:         1_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   100,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	for i := 0; i < 10; i++ {
		result, _ := mgr.AskPermission(ctx, "key"+strconv.Itoa(i), false, limiter_api.NoChange, limiter_api.NoChange)
		if result != limiter_api.Approved {
			t.Errorf("expected Approved, got %v", result)
		}
	}

	if time.Since(t0) > 1000*time.Millisecond {
		t.Errorf("expected to return immediately, but took %v", time.Since(t0))
	}
}

func TestLimiterManager_AskPermission_1m_requests(t *testing.T) {

	nRequests := 1_000_000
	nGoRoutines := 100

	globalCfg := &limiter_api.Config{
		WindowMillis:         1_000,
		MaxRequestsPerWindow: nRequests,
		MaxRequestsInQueue:   nRequests,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()
	t0 := time.Now()

	successes := atomic.Int32{}

	lop.ForEach(lo.Range(nGoRoutines), func(_ int, _ int) {
		for i := 0; i < nRequests/nGoRoutines; i++ {
			key := fmt.Sprintf("key-%d", rand.Intn(100))
			result, _ := mgr.AskPermission(ctx, key, false, limiter_api.NoChange, limiter_api.NoChange)
			if result != limiter_api.Approved {
				t.Errorf("expected Approved, got %v", result)
			} else {
				successes.Add(1)
			}
		}
	})

	if successes.Load() != int32(nRequests) {
		t.Errorf("expected %d successes, got %d", nRequests, successes.Load())
	}

	// Takes 1s on apple laptop, but 10-20s on GitHub actions :D
	if time.Since(t0) > 30*time.Second {
		t.Errorf("expected to return within 30s, but took %v", time.Since(t0))
	}

}

func TestLimiterManager_change_config(t *testing.T) {

	logging.ConfigureDefaultLogger("text", "debug", false)
	defer logging.ConfigureDefaultLogger("text", "info", false)

	globalCfg := &limiter_api.Config{
		WindowMillis:         1_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   0,
	}

	configChangeChan := make(chan *config.CfgFromFile, 1)

	mgr := NewManagerSet(globalCfg, nil, configChangeChan, DefaultSharding)
	defer mgr.Close()

	ctx := context.Background()

	result, _ := mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Approved {
		t.Fatalf("expected Approved, got %v", result)
	}

	result, _ = mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Denied {
		t.Fatalf("expected Denied, got %v", result)
	}

	// Update the configuration for a different key
	configChangeChan <- &config.CfgFromFile{
		Keys: []config.CfgFromFileKey{
			{
				KeyPattern:           "key2",
				KeyPatternIsRegex:    false,
				MaxRequestsPerWindow: 10000,
				MaxRequestsInQueue:   10000,
				WindowMillis:         10000,
			},
		},
	}

	time.Sleep(100 * time.Millisecond)

	result, _ = mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Denied {
		t.Fatalf("expected Denied, got %v", result)
	}

	// Update the configuration for our key
	configChangeChan <- &config.CfgFromFile{
		Keys: []config.CfgFromFileKey{
			{
				KeyPattern:           "key",
				KeyPatternIsRegex:    false,
				MaxRequestsPerWindow: 10000,
				MaxRequestsInQueue:   10000,
				WindowMillis:         10000,
			},
		},
	}

	time.Sleep(100 * time.Millisecond)

	result, _ = mgr.AskPermission(ctx, "key", false, limiter_api.NoChange, limiter_api.NoChange)
	if result != limiter_api.Approved {
		t.Fatalf("expected Approved, got %v", result)
	}

}

func TestLimiterManagerSet_GetShardIndex(t *testing.T) {

	logging.ConfigureDefaultLogger("text", "warn", false)
	defer logging.ConfigureDefaultLogger("text", "info", false)

	globalCfg := &limiter_api.Config{
		WindowMillis:         1_000,
		MaxRequestsPerWindow: 1,
		MaxRequestsInQueue:   0,
	}
	mgr := NewManagerSet(globalCfg, nil, nil, DefaultSharding)
	defer mgr.Close()

	nKeys := 100

	shardIndexCounts := map[int]int{}

	for i := 0; i < nKeys; i++ {
		key := fmt.Sprintf("key-%d", i)
		shardIndex := mgr.GetShardIndex(key)
		shardIndexCounts[shardIndex]++
		//slog.Info("shard info", "key", key, "shardIndex", shardIndex)
	}

	// We expect them to be somewhat evenly distributed
	if len(shardIndexCounts) != DefaultSharding {
		t.Fatalf("expected %d shard indexes, got %d", DefaultSharding, len(shardIndexCounts))
	}
}
