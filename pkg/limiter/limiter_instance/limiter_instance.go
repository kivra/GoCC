package limiter_instance

import (
	"context"
	"fmt"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_instance_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_manager_api"
	"github.com/kivra/gocc/pkg/logging/logctx"
	"github.com/samber/lo"
	"log/slog"
	"time"
)

func New(
	key string,
	config *limiter_api.Config,
	parent chan<- limiter_manager_api.Request,
) chan<- limiter_instance_api.Request {
	l := &internalState{
		key:                 key,
		config:              *config, // copy it, because we might change it internally
		nApprovedThisWindow: 0,
		timeLastUsed:        time.Now(),
		throttled:           make([]*limiter_api.PermissionRequest, 0, config.MaxRequestsPerWindow),

		mailbox: make(chan limiter_instance_api.Request, min(1_000, config.MaxRequestsPerWindow)), // some reasonable number
		parent:  parent,
	}

	go l.loop()

	return l.mailbox
}

type internalState struct {
	key                 string
	config              limiter_api.Config // Must be a copy, because we might change it internally for this instance
	nApprovedThisWindow int
	nDeniedThisWindow   int
	timeLastUsed        time.Time
	throttled           []*limiter_api.PermissionRequest // requests that have been received, but are being throttled/waiting

	mailbox chan limiter_instance_api.Request
	parent  chan<- limiter_manager_api.Request
}

func (state *internalState) flushQueued(ctx context.Context, nMax int) {
	n := min(nMax, len(state.throttled))
	if n > 0 {
		// slog.Debug(fmt.Sprintf("Flushing %d queued", n), logctx.GetAll(ctx)...)
		state.timeLastUsed = time.Now()
		numToFlush := min(len(state.throttled), n)
		for i := 0; i < numToFlush; i++ {
			state.throttled[i].RespChan <- &limiter_api.PermissionResponse{RespCode: limiter_api.Approved}
		}
		state.nApprovedThisWindow += numToFlush
		state.throttled = discardFirstItems(state.throttled, numToFlush)
	}
}

func (state *internalState) loop() {
	ctx := logctx.Add(context.Background(), "key", state.key)

	defer func() {
		slog.Info("Stopped limiter instance", logctx.GetAll(ctx)...)
	}()

	// create a ticker that ticks every windowMillis
	ticker := time.NewTicker(time.Duration(state.config.WindowMillis) * time.Millisecond)
	defer ticker.Stop()

	slog.Debug("Started limiter instance", logctx.GetAll(ctx)...)
	expiryNotificationSent := false

	for {
		select {
		// Reset approval count every time window
		case <-ticker.C:

			// slog.Debug("Resetting approval count", logctx.GetAll(ctx)...)
			state.nApprovedThisWindow = 0
			state.nDeniedThisWindow = 0
			state.flushQueued(ctx, state.config.MaxRequestsPerWindow) // also updates timeLastUsed if any were flushed
			if time.Since(state.timeLastUsed) > time.Duration(3*state.config.WindowMillis)*time.Millisecond && !expiryNotificationSent {
				// slog.Debug("instance expired: telling parent", logctx.GetAll(ctx)...)
				state.parent <- &limiter_manager_api.InstanceExpiredNotification{Key: state.key, InstanceMailbox: state.mailbox}
				expiryNotificationSent = true // important in high load scenarios, and where the manager is overloaded
			}

		// Receiving a Request, deciding if to approve or not
		case req := <-state.mailbox:

			switch r := req.(type) {

			case *limiter_instance_api.ConfigUpdateNotification:
				// slog.Debug("Received config update", logctx.GetAll(ctx)...)

				if r.WindowMillis != 0 &&
					r.WindowMillis != limiter_api.NoChange &&
					state.config.WindowMillis != r.WindowMillis {

					// slog.Debug(fmt.Sprintf("Changing windowMillis to %d", r.WindowMillis), logctx.GetAll(ctx)...)
					ticker.Stop()
					ticker = time.NewTicker(time.Duration(r.WindowMillis) * time.Millisecond)
					state.config.WindowMillis = r.WindowMillis
				}

				if r.MaxRequestsInQueue != 0 &&
					r.MaxRequestsInQueue != limiter_api.NoChange &&
					state.config.MaxRequestsInQueue != r.MaxRequestsInQueue {

					// slog.Debug(fmt.Sprintf("Changing MaxRequestsInQueue to %d", r.MaxRequestsInQueue), logctx.GetAll(ctx)...)
					state.config.MaxRequestsInQueue = r.MaxRequestsInQueue
				}

				if r.MaxRequestsPerWindow != 0 &&
					r.MaxRequestsPerWindow != limiter_api.NoChange &&
					state.config.MaxRequestsPerWindow != r.MaxRequestsPerWindow {

					// slog.Debug(fmt.Sprintf("Changing MaxRequestsPerWindow to %d", r.MaxRequestsPerWindow), logctx.GetAll(ctx)...)
					state.config.MaxRequestsPerWindow = r.MaxRequestsPerWindow
				}

			case *limiter_instance_api.Kill:
				// slog.Debug("Received kill notification, no more requests will be received by this instance", logctx.GetAll(ctx)...)
				state.flushQueued(ctx, len(state.throttled)) // flush any remaining requests. This can happen if we get messages EXACTLY when we're deregistered. It's ok. It's just a rate limiter :)
				state.parent <- &limiter_manager_api.InstanceDiedNotification{Key: state.key}
				return // we're done

			case *limiter_api.ClientGaveUpNotification:
				ctx := r.OriginalRequest.Ctx
				// slog.Debug("Client gave up, removing from queue", logctx.GetAll(ctx)...)
				// This is probably a little inefficient :). We should probably keep them in the queue
				// and just flag them as "gave up", and then when we flush the queue, we remove them.
				// But this is simpler and works fine, for now
				_, idx, found := lo.FindIndexOf(state.throttled, func(item *limiter_api.PermissionRequest) bool {
					return item.ReqID == r.OriginalRequest.ReqID
				})
				if found {
					state.throttled = discardItemAt(state.throttled, idx)
					// slog.Debug("Client gave up, removed from queue", logctx.GetAll(ctx)...)
				} else {
					slog.Warn("Client gave up, but original request was not found in queue for cleanup!", logctx.GetAll(ctx)...)
				}

			case *limiter_api.PermissionRequest:
				// ctx := r.Ctx // this + debug logging is a bit expensive, so we'll skip it for now
				// for limiter_instances. This is at the lowest level, and we also don't want to log too much.

				state.timeLastUsed = time.Now()

				if r.MaxRequests != limiter_api.NoChange {
					state.config.MaxRequestsPerWindow = r.MaxRequests
				}

				if r.MaxRequestsInQueue != limiter_api.NoChange {
					state.config.MaxRequestsInQueue = r.MaxRequestsInQueue
				}

				// check if we have any slots left
				if state.nApprovedThisWindow >= state.config.MaxRequestsPerWindow {
					if r.CanWait {
						if len(state.throttled) < state.config.MaxRequestsInQueue {
							// slog.Debug("No slots left in window, placing in wait queue", logctx.GetAll(ctx)...)
							state.throttled = append(state.throttled, r)
						} else {
							// slog.Debug("No slots left in window, and no slots left in wait queue, denying Request", logctx.GetAll(ctx)...)
							state.nDeniedThisWindow++
							r.RespChan <- &limiter_api.PermissionResponse{RespCode: limiter_api.Denied}
						}
					} else {
						// slog.Debug("No slots left in window, denying Request", logctx.GetAll(ctx)...)
						state.nDeniedThisWindow++
						r.RespChan <- &limiter_api.PermissionResponse{RespCode: limiter_api.Denied}
					}
				} else {
					// slog.Debug("Slot approved", logctx.GetAll(ctx)...)
					state.nApprovedThisWindow++
					r.RespChan <- &limiter_api.PermissionResponse{RespCode: limiter_api.Approved}
				}

			case *limiter_api.ReleaseRequest:
				// ctx := r.Ctx // this + debug logging is a bit expensive, so we'll skip it for now
				// for limiter_instances. This is at the lowest level, and we also don't want to log too much.

				// TODO: We should perhaps(?) only decrease the approved count if the request was approved this window.
				// another option is to treat release semantics entirely different, with transactions, and use some kind
				// of release guarantee, idk... maybe websockets and auto release on disconnect?
				// Or maybe have a separate counter and auto releas timer setup for transactions

				state.timeLastUsed = time.Now()
				state.nApprovedThisWindow = max(0, state.nApprovedThisWindow-1)

			case *limiter_api.DebugSnapshotRequest:
				// slog.Debug("Received debug snapshot request", logctx.GetAll(ctx)...)
				r.RespChan <- &limiter_api.InstanceDebugSnapshot{
					Key:                   state.key,
					Config:                state.config, // a copy
					NumApprovedThisWindow: state.nApprovedThisWindow,
					NumDeniedThisWindow:   state.nDeniedThisWindow,
					NumWaiting:            len(state.throttled),
					Found:                 true,
				}

			default:
				slog.Error(fmt.Sprintf("Unexpected message of type %T", req), logctx.GetAll(ctx)...)
			}
		}

	}

}

// discardFirstItems discards the first n elements from a slice, but the same slice
// is used. This should be used if we don't want a lot of 'dangling heads',
// see https://100go.co/#slices-and-memory-leaks-26
func discardFirstItems[T any](slice []T, n int) []T {
	if n == 0 {
		return slice
	}

	if n < 0 {
		panic("n is less than 0")
	}

	if n > len(slice) {
		panic("n is greater than the length of the slice")
	}

	for i := 0; i < len(slice)-n; i++ {
		slice[i] = slice[i+n]
	}

	return slice[:len(slice)-n]
}

// discardItemAt discards the element at index i from a slice. It does this by
// shifting all elements after i one step to the left. Then, we return the
// same memory block but with a length that is one less than before.
func discardItemAt[T any](slice []T, i int) []T {
	if i < 0 {
		panic("i is less than 0")
	}

	if i >= len(slice) {
		panic("i is greater than the length of the slice")
	}

	for ; i < len(slice)-1; i++ {
		slice[i] = slice[i+1]
	}

	return slice[:len(slice)-1]
}
