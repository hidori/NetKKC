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
		tag, kind, size := protowire.ConsumeTag(data)
		if size < 0 {
			return nil, protowire.ParseError(size)
		}

		data = data[size:]
		fld := field{number: tag}

		switch kind {
		case protowire.VarintType:
			fld.value, size = protowire.ConsumeVarint(data)
		case protowire.BytesType:
			fld.data, size = protowire.ConsumeBytes(data)
		case protowire.StartGroupType:
			fld.data, size = protowire.ConsumeGroup(tag, data)
		default:
			size = protowire.ConsumeFieldValue(tag, kind, data)
		}

		if size < 0 {
			return nil, fmt.Errorf("mozc protobuf field %d: %w", tag, protowire.ParseError(size))
		}

		fields = append(fields, fld)
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

	for _, fld := range fields {
		switch fld.number {
		case 1:
			result.id = fld.value
		case 5:
			preedit, err := decode(fld.data)
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

				for _, fld := range fields {
					switch fld.number {
					case 3:
						segment.focused = fld.value == 2
					case 4:
						segment.text = string(fld.data)
					case 6:
						segment.reading = string(fld.data)
					}
				}

				result.segments = append(result.segments, segment)
			}
		case 9:
			result.config = fld.data
		case 11:
			if fld.value != 0 {
				return output{}, fmt.Errorf("mozc session failure (%d)", fld.value)
			}
		case 14:
			list, err := decode(fld.data)
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

				var cand candidate

				for _, fld := range fields {
					switch fld.number {
					case 1:
						cand.id = int32(fld.value)
					case 4:
						cand.text = string(fld.data)
					}
				}

				result.candidates = append(result.candidates, cand)
			}
		}
	}

	return result, nil
}
