package glowflow

import (
	"context"

	"github.com/zhiyunliu/glue/log"
)

func GetLogger(ctx context.Context) log.Logger {
	if ctx == nil {
		return log.DefaultLogger
	}

	logger, ok := log.FromContext(ctx)
	if !ok {
		return log.DefaultLogger
	}
	return logger
}
