package goja

import "time"

type Config struct {
	MaxExecTime         time.Duration
	Properties          map[string]string
	InitScript          string
	FromVars            map[string]any
	UserdefinedFunction map[string]any
}
