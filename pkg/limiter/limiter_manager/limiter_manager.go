package limiter_manager

import (
	"context"
	"fmt"
	"github.com/kivra/gocc/pkg/config"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_instance"
	"github.com/kivra/gocc/pkg/limiter/limiter_instance_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_manager_api"
	"github.com/kivra/gocc/pkg/logging/logctx"
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"
	"hash/fnv"
	"log/slog"
	"sync/atomic"
	"time"
)

var DefaultSharding = 25

// LimiterManagerSet is the main entry point for the limiter system.
// It internally runs multiple shards of the manager, and uses hashing on keys
// to distribute requests to the correct shard. Below each shard is then
// a number of limiter instances, which are the actual rate limiters. There is one instance
// per key, and the manager shard is responsible for creating and managing these instances.
type LimiterManagerSet struct {
	reqIdGen  atomic.Int64
	mailboxes []chan<- limiter_manager_api.Request
}

func NewManagerSet(
	globalConfig *limiter_api.Config,
	initConfigFromFile *config.CfgFromFile,
	configFromFileCh <-chan *config.CfgFromFile,
	sharding int,
) *LimiterManagerSet {

	if sharding <= 0 {
		panic(fmt.Sprintf("BUG: sharding must be > 0, got %d", sharding))
	}
	if sharding > 100 {
		panic(fmt.Sprintf("BUG: sharding must be <= 100, got %d", sharding))
	}

	// Prepare the shards
	mailBoxes := make([]chan<- limiter_manager_api.Request, sharding)
	configChs := make([]chan *config.CfgFromFile, sharding)
	for i := 0; i < sharding; i++ {
		mailbox := make(chan limiter_manager_api.Request, 10_000) // some reasonable number of requests buffered in each manager
		configChs[i] = make(chan *config.CfgFromFile, 10)         // some reasonable number of config updates buffered in each manager
		mailBoxes[i] = mailbox
		go loop(globalConfig, mailbox, initConfigFromFile, configChs[i])
	}

	// Forward the changes in config from file to all shards
	// It's a bit ugly, but works. We don't know which shard is responsible
	// for which key, so we just forward it to all of them.
	go func() {
		for newCfg := range configFromFileCh {
			for _, ch := range configChs {
				ch <- newCfg
			}
		}
	}()

	l := &LimiterManagerSet{mailboxes: mailBoxes}

	return l
}

func hash(s string) uint32 {
	h := fnv.New32a()
	_, err := h.Write([]byte(s))
	if err != nil {
		panic(fmt.Sprintf("BUG: Failed to hash %s", s))
	}
	return h.Sum32()
}

// GetShardIndex returns the index of the shard that should handle the given key. Public for testing purposes.
func (mgr *LimiterManagerSet) GetShardIndex(key string) int {
	return int(hash(key)) % len(mgr.mailboxes)
}

func (mgr *LimiterManagerSet) getShardMailbox(key string) chan<- limiter_manager_api.Request {
	return mgr.mailboxes[mgr.GetShardIndex(key)]
}

// GetDebugSnapshot returns a debug snapshot for a specific key. It's generally not used in production,
// but during debugging and testing.
func (mgr *LimiterManagerSet) GetDebugSnapshot(key string) *limiter_api.InstanceDebugSnapshot {
	respChan := make(chan *limiter_api.InstanceDebugSnapshot, 1)

	req := &limiter_api.DebugSnapshotRequest{
		Key:      key,
		RespChan: respChan,
	}

	mailbox := mgr.getShardMailbox(key)

	mailbox <- req

	select {
	case resp := <-respChan:
		if resp != nil && resp.Found {
			return resp
		} else {
			slog.Warn("Instance not found", "key", key)
			return resp
		}
	case <-time.After(5 * time.Second):
		slog.Error("Gave up waiting for debug snapshot response", "key", key)
		return nil
	}
}

// GetDebugSnapshotsAll returns a debug snapshot for all instances. It's generally not used in production,
// but during debugging and testing.
func (mgr *LimiterManagerSet) GetDebugSnapshotsAll() *limiter_api.DebugSnapshotAll {

	many := lop.Map(mgr.mailboxes, func(mailbox chan<- limiter_manager_api.Request, _ int) *limiter_api.DebugSnapshotAll {

		respChan := make(chan *limiter_api.DebugSnapshotAll, len(mgr.mailboxes))
		req := &limiter_api.DebugSnapshotAllRequest{RespChan: respChan}
		mailbox <- req

		select {
		case resp := <-respChan:
			return resp
		case <-time.After(10 * time.Second):
			slog.Error("Gave up waiting for debug snapshots all response")
			return nil
		}
	})

	// Combine the many snapshots into one
	combined := many[0]
	for _, snap := range many[1:] {
		for key, val := range snap.Instances {
			combined.Instances[key] = val
		}
	}

	return combined
}

func (mgr *LimiterManagerSet) AskPermission(
	ctx context.Context,
	key string,
	canWait bool,
	maxRequests int,
	maxRequestsInQueue int,
) (limiter_api.ExtRespCode, string) {

	// Need a buffered channel (,1), so that the limiter can answer if the
	// client gives up before the limiter has had time to answer.
	respChan := make(chan *limiter_api.PermissionResponse, 1)

	// we used to use uuids here, but it's not necessary, an int64 atomic counter is enough
	// and 100x faster (YES we were actually peformance limited by UUID generation)
	reqId := fmt.Sprintf("%d", mgr.reqIdGen.Add(1))
	req := &limiter_api.PermissionRequest{
		ReqID:              reqId,
		Key:                key,
		RespChan:           respChan,
		Ctx:                ctx,
		CanWait:            canWait,
		MaxRequests:        maxRequests,
		MaxRequestsInQueue: maxRequestsInQueue,
	}

	mailbox := mgr.getShardMailbox(key)

	mailbox <- req

	select {
	case resp := <-respChan:
		return resp.RespCode, reqId
	case <-ctx.Done():
		slog.Warn("client gave up on request. context cancelled before receiving response", logctx.GetAll(ctx)...)
		mailbox <- &limiter_api.ClientGaveUpNotification{OriginalRequest: req}
		return limiter_api.ClientGaveUp, reqId
	}
}

// Release releases a previously acquired permission.
// The reqId is just for informational purposes. The key's bucket will get one more slot regardless of specified key.
// There is also no waiting for the release to be processed, but the release is guaranteed to be processed before
// the next permission request for the same key.
func (mgr *LimiterManagerSet) Release(
	ctx context.Context,
	key string,
	reqId string,
) {

	req := &limiter_api.ReleaseRequest{
		ReqID: reqId,
		Key:   key,
		Ctx:   ctx,
	}

	mailbox := mgr.getShardMailbox(key)

	mailbox <- req
}

func (mgr *LimiterManagerSet) Close() {
	slog.Info("Killing limiter manager, and all of its limiters")
	for _, mailbox := range mgr.mailboxes {
		mailbox <- &limiter_manager_api.Kill{}
	}
}

func combineConfigs(
	globalConfig *limiter_api.Config,
	configFromFile *config.CfgFromFileKey,
) *limiter_api.Config {
	result := *globalConfig
	if configFromFile != nil {
		if configFromFile.MaxRequestsInQueue != 0 { // 0 = not set
			result.MaxRequestsInQueue = configFromFile.MaxRequestsInQueue
		}
		if configFromFile.MaxRequestsPerWindow != 0 { // 0 = not set
			result.MaxRequestsPerWindow = configFromFile.MaxRequestsPerWindow
		}
		if configFromFile.WindowMillis != 0 { // 0 = not set
			result.WindowMillis = configFromFile.WindowMillis
		}
	}
	return &result
}

// mergeConfigsForInstance merges the global config with the config from the file
// Note that multiple configurations in the file can match the same key, so we need to merge them in order.
// For example, we might first have a regex matching all instances (key=".*") and then a specific key match (key="foo").
// The resulting config should be the combination of the two, applied in the order they were found.
func mergeConfigsForInstance(
	Key string,
	globalConfig *limiter_api.Config,
	configFromFile *config.CfgFromFile,
) *limiter_api.Config {
	result := globalConfig
	if configFromFile != nil {
		for _, configKey := range configFromFile.Keys {
			if configKey.MatchesKey(Key) {
				result = combineConfigs(result, &configKey)
			}
		}
	}
	return result
}

func loop(
	globalConfig *limiter_api.Config,
	mailbox chan limiter_manager_api.Request,
	configFromFile *config.CfgFromFile,
	configFromFileCh <-chan *config.CfgFromFile,
) {
	registry := map[string]chan<- limiter_instance_api.Request{}

	slog.Debug("Limiter manager started")

	for {
		select {
		case configFromFile = <-configFromFileCh:
			slog.Debug("Received new config from file, updating all instances...")
			for key, instance := range registry {
				// slog.Debug("Updating instance", "key", key)
				instanceConfig := mergeConfigsForInstance(key, globalConfig, configFromFile)
				instance <- &limiter_instance_api.ConfigUpdateNotification{Config: instanceConfig}
			}

		case req := <-mailbox:
			// slog.Debug("Received request", "req", fmt.Sprintf("%T", req))
			switch r := req.(type) {
			case *limiter_api.PermissionRequest:

				// Find an existing rate limiter instance or create a new one
				// and forward the request to it

				// slog.Debug("Received permission request", logctx.GetAll(r.Ctx)...)
				instance, exists := registry[r.Key]
				if !exists {
					instanceConfig := mergeConfigsForInstance(r.Key, globalConfig, configFromFile)
					instance = limiter_instance.New(r.Key, instanceConfig, mailbox)
					registry[r.Key] = instance
				}

				instance <- r

			case *limiter_api.ReleaseRequest:

				// Find an existing rate limiter instance

				// slog.Debug("Received ReleaseRequest request", logctx.GetAll(r.Ctx)...)
				instance, exists := registry[r.Key]
				if exists {
					instance <- r
				} else {
					slog.Warn("Received release request for unknown instance", logctx.GetAll(r.Ctx)...)
				}

			case *limiter_api.DebugSnapshotRequest:

				// slog.Debug("Received debug snapshot request", "key", r.Key)
				instance, exists := registry[r.Key]
				if !exists {
					slog.Warn("Received debug snapshot request for unknown instance", "key", r.Key)
					r.RespChan <- &limiter_api.InstanceDebugSnapshot{Found: false}
				} else {
					instance <- r
				}

			case *limiter_api.DebugSnapshotAllRequest:

				slog.Debug("Received debug snapshot all request")

				slog.Debug("Requesting snapshots from all instances")
				respChan := make(chan *limiter_api.InstanceDebugSnapshot, len(registry))
				for key, instance := range registry {
					instance <- &limiter_api.DebugSnapshotRequest{
						Key:      key,
						RespChan: respChan,
					}
				}

				slog.Debug("Waiting for snapshots from all instances in parallel")
				snapshots := lop.Map(lo.Keys(registry), func(key string, _ int) *limiter_api.InstanceDebugSnapshot {
					select {
					case resp := <-respChan:
						return resp
					case <-time.After(3 * time.Second):
						slog.Error("Gave up waiting for debug snapshot response for key", "key", key)
						return nil
					}
				})

				slog.Debug("Filtering out nil snapshots")
				snapshots = lo.Filter(snapshots, func(snap *limiter_api.InstanceDebugSnapshot, _ int) bool {
					return snap != nil
				})

				slog.Debug("Sending debug snapshot all response back")
				r.RespChan <- &limiter_api.DebugSnapshotAll{
					Instances: lo.SliceToMap(snapshots, func(snap *limiter_api.InstanceDebugSnapshot) (string, *limiter_api.InstanceDebugSnapshot) {
						return snap.Key, snap
					}),
				}

			case *limiter_api.ClientGaveUpNotification:

				// The client has disconnected, probably due to a client side timeout.
				// We should notify the instance so that it doesn't consume an approval slot in the queue.

				// slog.Debug("Received client gave up notification", logctx.GetAll(r.OriginalRequest.Ctx)...)
				instance, exists := registry[r.OriginalRequest.Key]
				if exists {
					instance <- r
				} else {
					slog.Warn("Received client gave up notification for unknown instance", logctx.GetAll(r.OriginalRequest.Ctx)...)
				}

			case *limiter_manager_api.InstanceExpiredNotification:

				// The limiter instance has been idle for a while and is probably not needed anymore.
				// We should remove it from the registry and notify the instance that no new messages can be sent to it,
				// so that it can stop. If any new messages later are sent to the same key, a new instance will be created.
				// see case *limiter_api.PermissionRequest above.

				// slog.Debug("Received instance expired notification, will de-register instance", "key", r.Key)
				// Deregister from system and notify the instance that it is safe for stopping,
				// i.e. it won't receive any more requests
				storedInstance, exists := registry[r.Key]
				if !exists {
					slog.Warn("Received instance expired notification for unknown instance", "key", r.Key)
				} else {
					// Can happen if multiple deregister messages are sent for the same instance
					if storedInstance != r.InstanceMailbox {
						slog.Warn("Received instance expired notification => instance mismatch", "key", r.Key)
					} else {
						delete(registry, r.Key)
					}
				}
				r.InstanceMailbox <- &limiter_instance_api.Kill{}

			case *limiter_manager_api.Kill:
				slog.Info("Close received, stopping all instances and limiter manager. This should probably only be used for testing")
				for _, instance := range registry {
					instance <- &limiter_instance_api.Kill{}
				}
				return

			case *limiter_manager_api.InstanceDiedNotification:
				// slog.Debug("Received instance died notification, this is received for debugging purposes", "key", r.Key)

			default:
				slog.Error(fmt.Sprintf("Received unknown request of unknown type %T", req))
			}
		}
	}
}
