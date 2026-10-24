package mozc

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/hidori/NetKKC/internal/errs"
)

var (
	// ErrBoundaries means Mozc cannot realize the requested segment boundaries.
	ErrBoundaries = errors.New("mozc cannot realize the requested segment boundaries")
	// ErrUnavailable means Mozc cannot be reached or failed to process a request.
	ErrUnavailable = errors.New("mozc is unavailable")
	// ErrInvalidResponse means Mozc returned a response that cannot be used.
	ErrInvalidResponse = errors.New("mozc returned an invalid response")
)

type Segment struct {
	Reading    string
	Candidates []string
}

// Converter serializes access to its dedicated Mozc server. Each conversion
// creates and deletes its own session; no session IDs cross the HTTP boundary.
type Converter struct {
	client *Client
	gate   chan struct{}
}

func NewConverter(client *Client) *Converter {
	return &Converter{client: client, gate: make(chan struct{}, 1)}
}

// Convert returns the segments of reading. When ctx is done before the
// conversion completes, it returns the context error.
func (c *Converter) Convert(ctx context.Context, reading string, boundaries []string) ([]Segment, error) {
	segments, err := c.convert(ctx, reading, boundaries)
	if err != nil && ctx.Err() != nil {
		return nil, errs.Wrap(ctx.Err())
	}

	return segments, err
}

func (c *Converter) convert(ctx context.Context, reading string, boundaries []string) ([]Segment, error) {
	release, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	sess, err := c.client.create(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.close(ctx)

	out, err := sess.begin(ctx, reading)
	if err != nil {
		return nil, err
	}
	// Mozc may rewrite the input (e.g. full-width ASCII to half-width), so its
	// reading becomes the reference for the rest of the conversion.
	reading = joined(out)
	if reading == "" {
		return nil, errs.Errorf("%w: no segments", ErrInvalidResponse)
	}

	if len(boundaries) > 0 {
		out, err = sess.applyBoundaries(ctx, out, reading, boundaries)
		if err != nil {
			return nil, err
		}
	}

	return sess.collectSegments(ctx, out, reading)
}

// acquire waits for exclusive use of the Mozc server and returns its release.
func (c *Converter) acquire(ctx context.Context) (func(), error) {
	select {
	case c.gate <- struct{}{}:
		return func() { <-c.gate }, nil
	case <-ctx.Done():
		return nil, errs.Wrap(ctx.Err())
	}
}

// applyBoundaries resizes the segments to boundaries and focuses the first one.
func (s *session) applyBoundaries(ctx context.Context, out commandOutput, reading string, boundaries []string) (commandOutput, error) {
	if strings.Join(boundaries, "") != reading {
		return commandOutput{}, ErrBoundaries
	}

	out, err := s.resize(ctx, out, reading, boundaries)
	if err != nil {
		return commandOutput{}, err
	}
	// Resize leaves focus on the final segment. Return to the first one.
	for focused(out) > 0 {
		previous := focused(out)

		out, err = s.key(ctx, number(nil, keySpecialKey, specialKeyLeft))
		if err != nil {
			return commandOutput{}, err
		}

		if focused(out) != previous-1 {
			return commandOutput{}, ErrBoundaries
		}
	}

	return out, nil
}

// collectSegments reads the candidates of every segment, moving the focus from
// the first segment to the last.
func (s *session) collectSegments(ctx context.Context, out commandOutput, reading string) ([]Segment, error) {
	result := make([]Segment, 0, len(out.segments))
	for i := range len(out.segments) {
		if focused(out) != i {
			return nil, errs.Errorf("%w: unexpected segment focus", ErrInvalidResponse)
		}

		segment, err := focusedSegment(out, i)
		if err != nil {
			return nil, err
		}

		result = append(result, segment)

		if i+1 < len(out.segments) {
			out, err = s.key(ctx, number(nil, keySpecialKey, specialKeyRight))
			if err != nil {
				return nil, err
			}

			if err := checkReading(out, reading); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// focusedSegment returns the reading and the unique candidates of the focused
// segment at index.
func focusedSegment(out commandOutput, index int) (Segment, error) {
	segment := Segment{Reading: out.segments[index].reading, Candidates: make([]string, 0)}

	seen := make(map[string]bool)
	for _, candidate := range out.candidates {
		if !seen[candidate.text] {
			segment.Candidates = append(segment.Candidates, candidate.text)
			seen[candidate.text] = true
		}
	}

	if len(segment.Candidates) == 0 {
		return Segment{}, errs.Errorf("%w: no candidates for a segment", ErrInvalidResponse)
	}

	return segment, nil
}

func (s *session) resize(ctx context.Context, out commandOutput, reading string, boundaries []string) (commandOutput, error) {
	for i, target := range boundaries {
		var err error

		out, err = s.resizeSegment(ctx, out, reading, i, target)
		if err != nil {
			return commandOutput{}, err
		}

		if i+1 < len(boundaries) {
			out, err = s.key(ctx, number(nil, keySpecialKey, specialKeyRight))
			if err != nil {
				return commandOutput{}, err
			}
		}
	}

	if len(out.segments) != len(boundaries) {
		return commandOutput{}, ErrBoundaries
	}

	for i, segment := range out.segments {
		if segment.reading != boundaries[i] {
			return commandOutput{}, ErrBoundaries
		}
	}

	return out, nil
}

// resizeSegment expands or shrinks the focused segment at index until its
// reading equals target.
func (s *session) resizeSegment(ctx context.Context, out commandOutput, reading string, index int, target string) (commandOutput, error) {
	for steps := 0; ; steps++ {
		if index >= len(out.segments) || focused(out) != index || steps > utf8.RuneCountInString(reading) {
			return commandOutput{}, ErrBoundaries
		}

		current := out.segments[index].reading
		if current == target {
			return out, nil
		}

		key := uint64(specialKeyRight) // SHIFT+RIGHT expands one Unicode code point in Mozc.
		if utf8.RuneCountInString(current) >= utf8.RuneCountInString(target) {
			key = specialKeyLeft // SHIFT+LEFT shrinks one Unicode code point.
		}

		var err error

		out, err = s.key(ctx, number(number(nil, keySpecialKey, key), keyModifierKeys, modifierShift))
		if err != nil {
			return commandOutput{}, err
		}

		if err := checkReading(out, reading); err != nil {
			return commandOutput{}, err
		}

		if index >= len(out.segments) || out.segments[index].reading == current {
			return commandOutput{}, ErrBoundaries
		}
	}
}

func focused(out commandOutput) int {
	for i, segment := range out.segments {
		if segment.focused {
			return i
		}
	}

	return -1
}

func joined(out commandOutput) string {
	var text strings.Builder
	for _, segment := range out.segments {
		text.WriteString(segment.reading)
	}

	return text.String()
}

func checkReading(out commandOutput, reading string) error {
	if joined(out) != reading {
		return errs.Errorf("%w: reading changed during conversion", ErrInvalidResponse)
	}

	return nil
}
