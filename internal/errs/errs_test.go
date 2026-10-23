package errs_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/hidori/NetKKC/internal/errs"
)

func TestErrors(t *testing.T) {
	t.Parallel()

	traced := errs.Wrap(io.EOF)

	for _, test := range []struct {
		name    string
		err     error
		message string
		is      error
	}{
		{"new", errs.New("boom"), "boom", nil},
		{"errorf", errs.Errorf("read: %w", io.EOF), "read: EOF", io.EOF},
		{"wrap", errs.Wrap(io.EOF), "EOF", io.EOF},
		{"wrapf", errs.Wrapf(io.EOF, "read %d", 1), "read 1: EOF", io.EOF},
		{"wrapf traced", errs.Wrapf(traced, "read"), "read: EOF", io.EOF},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if test.err.Error() != test.message {
				t.Fatalf("message = %q, want %q", test.err.Error(), test.message)
			}

			if test.is != nil && !errors.Is(test.err, test.is) {
				t.Fatalf("%v does not wrap %v", test.err, test.is)
			}

			if trace := fmt.Sprintf("%+v", test.err); !strings.Contains(trace, "errs_test.TestErrors\n") {
				t.Fatalf("stack trace does not start at the caller:\n%s", trace)
			}
		})
	}
}

func TestWrapNil(t *testing.T) {
	t.Parallel()

	if errs.Wrap(nil) != nil || errs.Wrapf(nil, "message") != nil {
		t.Fatal("wrapping nil must return nil")
	}
}
