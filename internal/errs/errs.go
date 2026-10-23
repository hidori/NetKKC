// Package errs creates and wraps errors with the stack trace of the call.
package errs

import (
	"errors"
	"fmt"
	"io"
	"runtime"
)

const maxDepth = 32

type stackError struct {
	err   error
	stack []uintptr
}

// New returns an error with message and the caller's stack trace.
func New(message string) error {
	return withStack(errors.New(message), nil)
}

// Errorf formats an error like fmt.Errorf and records the caller's stack trace.
func Errorf(format string, args ...any) error {
	return withStack(fmt.Errorf(format, args...), nil)
}

// Wrap records the caller's stack trace on err. It returns nil for nil and
// keeps the existing stack trace when err already carries one.
func Wrap(err error) error {
	if err == nil {
		return nil
	}

	return withStack(err, traced(err))
}

// Wrapf prefixes err with a formatted message and records the caller's stack
// trace. It returns nil for nil and keeps the existing stack trace when err
// already carries one.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}

	return withStack(fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err), traced(err))
}

func traced(err error) *stackError {
	inner, _ := errors.AsType[*stackError](err)

	return inner
}

// withStack must be called directly from an exported function so that the
// recorded stack starts at that function's caller.
func withStack(err error, inner *stackError) error {
	if inner != nil {
		return &stackError{err: err, stack: inner.stack}
	}

	pcs := make([]uintptr, maxDepth)
	count := runtime.Callers(3, pcs)

	return &stackError{err: err, stack: pcs[:count]}
}

func (e *stackError) Error() string {
	return e.err.Error()
}

func (e *stackError) Unwrap() error {
	return e.err
}

// Format prints the stack trace after the message for %+v.
func (e *stackError) Format(state fmt.State, verb rune) {
	_, _ = io.WriteString(state, e.err.Error())

	if verb != 'v' || !state.Flag('+') {
		return
	}

	frames := runtime.CallersFrames(e.stack)
	for {
		frame, more := frames.Next()
		_, _ = fmt.Fprintf(state, "\n%s\n\t%s:%d", frame.Function, frame.File, frame.Line)

		if !more {
			return
		}
	}
}
