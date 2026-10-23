package mozc

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// This package implements the subset of Mozc's proto2 IPC used by conversion.
// Field numbers and command values follow Mozc 2.29.5160.102:
// https://github.com/google/mozc/tree/2.29.5160.102/src/protocol
// https://github.com/google/mozc/blob/2.29.5160.102/src/ipc/ipc.proto
// Unknown protobuf fields are skipped to allow compatible newer servers.
type field struct {
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

func decode(data []byte) ([]field, error) {
	var fields []field

	for len(data) > 0 {
		tag, kind, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}

		data = data[n:]
		f := field{number: tag}

		switch kind {
		case protowire.VarintType:
			f.value, n = protowire.ConsumeVarint(data)
		case protowire.BytesType:
			f.data, n = protowire.ConsumeBytes(data)
		case protowire.StartGroupType:
			f.data, n = protowire.ConsumeGroup(tag, data)
		default:
			n = protowire.ConsumeFieldValue(tag, kind, data)
		}

		if n < 0 {
			return nil, fmt.Errorf("mozc protobuf field %d: %w", tag, protowire.ParseError(n))
		}

		fields = append(fields, f)
		data = data[n:]
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

type output struct {
	id         uint64
	segments   []preeditSegment
	candidates []candidate
	config     []byte
}

func decodeOutput(data []byte) (output, error) {
	fields, err := decode(data)
	if err != nil {
		return output{}, err
	}

	var result output

	for _, f := range fields {
		switch f.number {
		case 1:
			result.id = f.value
		case 5:
			preedit, err := decode(f.data)
			if err != nil {
				return output{}, err
			}

			for _, part := range preedit {
				if part.number != 2 {
					continue
				}

				fields, err := decode(part.data)
				if err != nil {
					return output{}, err
				}

				var segment preeditSegment

				for _, f := range fields {
					switch f.number {
					case 3:
						segment.focused = f.value == 2
					case 4:
						segment.text = string(f.data)
					case 6:
						segment.reading = string(f.data)
					}
				}

				result.segments = append(result.segments, segment)
			}
		case 9:
			result.config = f.data
		case 11:
			if f.value != 0 {
				return output{}, fmt.Errorf("mozc session failure (%d)", f.value)
			}
		case 14:
			list, err := decode(f.data)
			if err != nil {
				return output{}, err
			}

			for _, entry := range list {
				if entry.number != 2 {
					continue
				}

				fields, err := decode(entry.data)
				if err != nil {
					return output{}, err
				}

				var c candidate

				for _, f := range fields {
					switch f.number {
					case 1:
						c.id = int32(f.value)
					case 4:
						c.text = string(f.data)
					}
				}

				result.candidates = append(result.candidates, c)
			}
		}
	}

	return result, nil
}
