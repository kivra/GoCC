package logctx

import (
	"context"
	"log/slog"
)

func Add(c context.Context, key string, value string) context.Context {
	existing := GetAll(c)
	result := make([]any, 0, len(existing)+1)
	for _, v := range existing {
		slogAttr, ok := v.(slog.Attr)
		if ok && slogAttr.Key == key {
			continue
		}
		result = append(result, v)
	}
	result = append(result, slog.String(key, value))
	return context.WithValue(c, logCtxKey, result)
}

func GetAll(c context.Context) []any {
	prev := c.Value(logCtxKey)
	if prev != nil {
		return prev.([]any)
	} else {
		return []any{}
	}
}

type logCtxKeyT string

const logCtxKey logCtxKeyT = "log_ctx"
