package util

import "context"

type Alerter interface {
	Alert(ctx context.Context, format string, v ...interface{})
}

func NoopAlerter() Alerter {
	return noopAlerter{}
}

type noopAlerter struct{}

func (noopAlerter) Alert(ctx context.Context, format string, v ...interface{}) {}
