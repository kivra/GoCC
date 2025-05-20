package limiter_instance_api

import "github.com/kivra/gocc/pkg/limiter/limiter_api"

type Request interface {
	IsLimiterInstanceRequest()
}

type Kill struct {
}

func (r *Kill) IsLimiterInstanceRequest() {}

type ConfigUpdateNotification struct {
	*limiter_api.Config
}

func (r *ConfigUpdateNotification) IsLimiterInstanceRequest() {}
