package limiter_api

import (
	"context"
)

type ExtRespCode string

const (
	Approved     ExtRespCode = "approved"       // request was approved
	Denied       ExtRespCode = "denied"         // request was denied
	ClientGaveUp ExtRespCode = "client-gave-up" // client gave up/disconnected before getting a response
)

type Config struct {
	WindowMillis         int
	MaxRequestsPerWindow int
	MaxRequestsInQueue   int
}

type PermissionRequest struct {
	ReqID              string
	Key                string
	RespChan           chan *PermissionResponse
	Ctx                context.Context
	CanWait            bool
	MaxRequests        int
	MaxRequestsInQueue int
}

func (r *PermissionRequest) IsLimiterManagerRequest()  {}
func (r *PermissionRequest) IsLimiterInstanceRequest() {}

type ReleaseRequest struct {
	ReqID string // for logging purposes only
	Key   string
	Ctx   context.Context
}

func (r *ReleaseRequest) IsLimiterManagerRequest()  {}
func (r *ReleaseRequest) IsLimiterInstanceRequest() {}

type PermissionResponse struct {
	RespCode ExtRespCode
}

type ClientGaveUpNotification struct {
	OriginalRequest *PermissionRequest
}

func (r *ClientGaveUpNotification) IsLimiterManagerRequest()  {}
func (r *ClientGaveUpNotification) IsLimiterInstanceRequest() {}

const (
	NoChange = -1_000_000_000
)

type DebugSnapshotAllRequest struct {
	RespChan chan *DebugSnapshotAll
}

func (r *DebugSnapshotAllRequest) IsLimiterManagerRequest() {}

type DebugSnapshotRequest struct {
	Key      string
	RespChan chan *InstanceDebugSnapshot
}

func (r *DebugSnapshotRequest) IsLimiterManagerRequest()  {}
func (r *DebugSnapshotRequest) IsLimiterInstanceRequest() {}

type DebugSnapshotAll struct {
	Instances map[string]*InstanceDebugSnapshot
}

type InstanceDebugSnapshot struct {
	Key                   string
	Config                Config // needs to be a copy to avoid race conditions
	NumApprovedThisWindow int
	NumDeniedThisWindow   int
	NumWaiting            int
	Found                 bool // The instance was found
}
