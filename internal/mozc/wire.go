package mozc

import (
	"fmt"

	"github.com/hidori/NetKKC/internal/errs"
	"google.golang.org/protobuf/encoding/protowire"
)

// This package implements the subset of Mozc's proto2 IPC used by conversion.
// Field numbers and command values follow Mozc 2.29.5160.102:
// https://github.com/google/mozc/tree/2.29.5160.102/src/protocol
// https://github.com/google/mozc/blob/2.29.5160.102/src/ipc/ipc.proto
// Unknown protobuf fields are skipped to allow compatible newer servers.
type protoField struct {
	number protowire.Number
	value  uint64
	data   []byte
}

func number(dst []byte, tag protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, tag, protowire.VarintType)

	return protowire.AppendVarint(dst, value)
}

func message(dst []byte, tag protowire.Number, data []byte) []byte {
	dst = protowire.AppendTag(dst, tag, protowire.BytesType)

	return protowire.AppendBytes(dst, data)
}

func decode(data []byte) ([]protoField, error) {
	var fields []protoField

	for len(data) > 0 {
		tag, kind, size := protowire.ConsumeTag(data)
		if size < 0 {
			return nil, errs.Wrap(protowire.ParseError(size))
		}

		data = data[size:]
		field := protoField{number: tag}

		switch kind {
		case protowire.VarintType:
			field.value, size = protowire.ConsumeVarint(data)
		case protowire.BytesType:
			field.data, size = protowire.ConsumeBytes(data)
		case protowire.StartGroupType:
			field.data, size = protowire.ConsumeGroup(tag, data)
		default:
			size = protowire.ConsumeFieldValue(tag, kind, data)
		}

		if size < 0 {
			return nil, fmt.Errorf("mozc protobuf field %d: %w", tag, protowire.ParseError(size))
		}

		fields = append(fields, field)
		data = data[size:]
	}

	return fields, nil
}

type preeditSegment struct {
	reading string
	text    string
	focused bool
}

type candidate struct {
	id   int32
	text string
}

type commandOutput struct {
	id         uint64
	segments   []preeditSegment
	candidates []candidate
	config     []byte
}

func decodeOutput(data []byte) (commandOutput, error) {
	fields, err := decode(data)
	if err != nil {
		return commandOutput{}, err
	}

	var result commandOutput

	for _, field := range fields {
		switch field.number {
		case outputID:
			result.id = field.value
		case outputPreedit:
			segments, err := decodePreedit(field.data)
			if err != nil {
				return commandOutput{}, err
			}

			result.segments = append(result.segments, segments...)
		case outputConfig:
			result.config = field.data
		case outputErrorCode:
			if field.value != errorCodeSessionSuccess {
				return commandOutput{}, errs.Errorf("%w: session failure (%d)", ErrUnavailable, field.value)
			}
		case outputAllCandidateWords:
			candidates, err := decodeCandidateList(field.data)
			if err != nil {
				return commandOutput{}, err
			}

			result.candidates = append(result.candidates, candidates...)
		default: // Unknown fields are skipped.
		}
	}

	return result, nil
}

func decodePreedit(data []byte) ([]preeditSegment, error) {
	parts, err := decode(data)
	if err != nil {
		return nil, err
	}

	var segments []preeditSegment

	for _, part := range parts {
		if part.number != preeditSegmentGroup {
			continue
		}

		segment, err := decodePreeditSegment(part.data)
		if err != nil {
			return nil, err
		}

		segments = append(segments, segment)
	}

	return segments, nil
}

func decodePreeditSegment(data []byte) (preeditSegment, error) {
	fields, err := decode(data)
	if err != nil {
		return preeditSegment{}, err
	}

	var segment preeditSegment

	for _, field := range fields {
		switch field.number {
		case segmentAnnotation:
			segment.focused = field.value == annotationHighlight
		case segmentValue:
			segment.text = string(field.data)
		case segmentKey:
			segment.reading = string(field.data)
		default: // Unknown fields are skipped.
		}
	}

	return segment, nil
}

func decodeCandidateList(data []byte) ([]candidate, error) {
	entries, err := decode(data)
	if err != nil {
		return nil, err
	}

	var candidates []candidate

	for _, entry := range entries {
		if entry.number != candidateListCandidates {
			continue
		}

		cand, err := decodeCandidateWord(entry.data)
		if err != nil {
			return nil, err
		}

		candidates = append(candidates, cand)
	}

	return candidates, nil
}

func decodeCandidateWord(data []byte) (candidate, error) {
	fields, err := decode(data)
	if err != nil {
		return candidate{}, err
	}

	var cand candidate

	for _, field := range fields {
		switch field.number {
		case candidateWordID:
			cand.id = int32(field.value)
		case candidateWordValue:
			cand.text = string(field.data)
		default: // Unknown fields are skipped.
		}
	}

	return cand, nil
}
