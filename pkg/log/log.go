package log

import (
	"context"
	"strings"

	"github.com/giantswarm/micrologger"
	"github.com/go-logr/logr"
	"github.com/go-stack/stack"
)

type Logger struct {
	micrologger.Logger
	Verbosity int
	names     []string
}

func (m Logger) Enabled() bool {
	return m.Verbosity > 0
}

func (m Logger) Info(msg string, keysAndValues ...interface{}) {
	if m.Verbosity < 2 {
		return
	}
	m.withName().With(keysAndValues...).With("level", "info").Log("message", msg)
}

func (m Logger) Error(err error, msg string, keysAndValues ...interface{}) {
	if m.Verbosity < 1 {
		return
	}
	m.withName().With(keysAndValues...).Errorf(context.Background(), err, msg)
}

func (m Logger) withName() Logger {
	wrapperCopy := m
	if len(m.names) == 0 {
		wrapperCopy.Logger = m.Logger.With("name", strings.Join(m.names, "."))
	}
	wrapperCopy.Logger = m.Logger.With("caller", stack.Caller(3))
	return wrapperCopy
}

func (m Logger) V(level int) logr.Logger {
	wrapperCopy := m
	wrapperCopy.Verbosity = level
	return wrapperCopy
}

func (m Logger) WithValues(keysAndValues ...interface{}) logr.Logger {
	wrapperCopy := m
	wrapperCopy.Logger = m.Logger.With(keysAndValues...)
	return wrapperCopy
}

func (m Logger) WithName(name string) logr.Logger {
	wrapperCopy := m
	wrapperCopy.names = append(m.names[:], name)
	return wrapperCopy
}
