// Postman/Akita addition (not part of upstream glog).
//
// glog keeps its sink registry in an internal package, so code outside glog
// cannot receive log entries. This file registers one extra text sink at
// package init (the only time the registry may be modified) and lets callers
// plug a forwarding function into it later, e.g. to send every log entry to an
// APM agent. The stderr and file sinks are untouched, so stdout/stderr output
// is unchanged whether or not a forwarder is set.

package glog

import (
	"context"
	"sync/atomic"

	"github.com/golang/glog/internal/logsink"
)

// Forwarder receives each formatted log entry, header included, exactly as it
// is written to stderr. severity is "INFO", "WARNING", "ERROR" or "FATAL".
//
// ctx is the context passed to the *Context logging functions (InfoContext,
// ErrorContextf, ...), and nil for the others. A forwarder can use it to tie
// the entry to request-scoped data such as a trace. Like logsink.Meta.Context,
// it is only valid for the duration of the call.
//
// It runs synchronously on the goroutine that logged, after the stderr and file
// sinks have written the entry, so it must be fast and must not call glog. The
// line buffer is only valid for the duration of the call.
type Forwarder func(ctx context.Context, severity string, line []byte)

var forwarder atomic.Pointer[Forwarder]

// SetForwarder installs f as the forwarder, replacing any previous one. Pass
// nil to stop forwarding. Safe to call at any time, from any goroutine.
func SetForwarder(f Forwarder) {
	if f == nil {
		forwarder.Store(nil)
		return
	}
	forwarder.Store(&f)
}

type forwardSink struct{}

func (forwardSink) Enabled(*logsink.Meta) bool {
	return forwarder.Load() != nil
}

func (forwardSink) Emit(m *logsink.Meta, data []byte) (int, error) {
	if f := forwarder.Load(); f != nil {
		func() {
			defer func() { _ = recover() }()
			(*f)(m.Context, m.Severity.String(), data)
		}()
	}
	// Never report an error: a forwarding problem must not terminate the process.
	return len(data), nil
}

func init() {
	logsink.TextSinks = append(logsink.TextSinks, forwardSink{})
}
