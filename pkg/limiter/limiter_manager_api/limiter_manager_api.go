package limiter_manager_api

import (
	"github.com/kivra/gocc/pkg/limiter/limiter_instance_api"
)

type Request interface {
	IsLimiterManagerRequest()
}

type InstanceExpiredNotification struct {
	Key             string
	InstanceMailbox chan<- limiter_instance_api.Request
}

func (r *InstanceExpiredNotification) IsLimiterManagerRequest() {}

type Kill struct {
}

func (r *Kill) IsLimiterManagerRequest() {}

type InstanceDiedNotification struct {
	Key string
}

func (r *InstanceDiedNotification) IsLimiterManagerRequest() {}
