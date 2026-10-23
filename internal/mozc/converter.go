package mozc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var ErrBoundaries = errors.New("Mozc cannot realize the requested segment boundaries")

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

func (c *Converter) Convert(ctx context.Context, reading string, boundaries []string) ([]Segment, error) {
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s, err := c.client.create(ctx)
	if err != nil {
		return nil, err
	}
	defer s.close()
	out, err := s.begin(ctx, reading)
	if err != nil {
		return nil, err
	}
	if err := checkReading(out, reading); err != nil {
		return nil, err
	}
	if len(boundaries) > 0 {
		out, err = s.resize(ctx, out, reading, boundaries)
		if err != nil {
			return nil, err
		}
		// Resize leaves focus on the final segment. Return to the first one.
		for focused(out) > 0 {
			previous := focused(out)
			out, err = s.key(ctx, number(nil, 3, 6)) // LEFT
			if err != nil {
				return nil, err
			}
			if focused(out) != previous-1 {
				return nil, ErrBoundaries
			}
		}
	}
	result := make([]Segment, 0, len(out.segments))
	for i := 0; i < len(out.segments); i++ {
		if focused(out) != i {
			return nil, errors.New("unexpected Mozc segment focus")
		}
		segment := Segment{Reading: out.segments[i].reading, Candidates: make([]string, 0)}
		seen := make(map[string]bool)
		for _, candidate := range out.candidates {
			if !seen[candidate.text] {
				segment.Candidates = append(segment.Candidates, candidate.text)
				seen[candidate.text] = true
			}
		}
		if len(segment.Candidates) == 0 {
			return nil, errors.New("Mozc returned no candidates for a segment")
		}
		result = append(result, segment)
		if i+1 < len(out.segments) {
			out, err = s.key(ctx, number(nil, 3, 7)) // RIGHT
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

func (s *session) resize(ctx context.Context, out output, reading string, boundaries []string) (output, error) {
	for i, target := range boundaries {
		for steps := 0; ; steps++ {
			if i >= len(out.segments) || focused(out) != i || steps > utf8.RuneCountInString(reading) {
				return output{}, ErrBoundaries
			}
			current := out.segments[i].reading
			if current == target {
				break
			}
			key := uint64(7) // SHIFT+RIGHT expands one Unicode code point in Mozc.
			if utf8.RuneCountInString(current) >= utf8.RuneCountInString(target) {
				key = 6 // SHIFT+LEFT shrinks one Unicode code point.
			}
			var err error
			out, err = s.key(ctx, number(number(nil, 3, key), 4, 4))
			if err != nil {
				return output{}, err
			}
			if err := checkReading(out, reading); err != nil {
				return output{}, err
			}
			if i >= len(out.segments) || out.segments[i].reading == current {
				return output{}, ErrBoundaries
			}
		}
		if i+1 < len(boundaries) {
			var err error
			out, err = s.key(ctx, number(nil, 3, 7))
			if err != nil {
				return output{}, err
			}
		}
	}
	if len(out.segments) != len(boundaries) {
		return output{}, ErrBoundaries
	}
	for i, segment := range out.segments {
		if segment.reading != boundaries[i] {
			return output{}, ErrBoundaries
		}
	}
	return out, nil
}

func focused(out output) int {
	for i, segment := range out.segments {
		if segment.focused {
			return i
		}
	}
	return -1
}

func checkReading(out output, reading string) error {
	var text strings.Builder
	for _, segment := range out.segments {
		if segment.reading == "" {
			return errors.New("Mozc returned an empty segment reading")
		}
		text.WriteString(segment.reading)
	}
	if len(out.segments) == 0 || text.String() != reading {
		return fmt.Errorf("Mozc did not preserve the input reading")
	}
	return nil
}
