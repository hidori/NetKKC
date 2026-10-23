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
			return nil, protowire.ParseError(size)
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
		case 1:
			result.id = field.value
		case 5:
			preedit, err := decode(field.data)
			if err != nil {
				return commandOutput{}, err
			}

			for _, part := range preedit {
				if part.number != 2 {
					continue
				}

				fields, err := decode(part.data)
				if err != nil {
					return commandOutput{}, err
				}

				var segment preeditSegment

				for _, field := range fields {
					switch field.number {
					case 3:
						segment.focused = field.value == 2
					case 4:
						segment.text = string(field.data)
					case 6:
						segment.reading = string(field.data)
					}
				}

				result.segments = append(result.segments, segment)
			}
		case 9:
			result.config = field.data
		case 11:
			if field.value != 0 {
				return commandOutput{}, fmt.Errorf("mozc session failure (%d)", field.value)
			}
		case 14:
			list, err := decode(field.data)
			if err != nil {
				return commandOutput{}, err
			}

			for _, entry := range list {
				if entry.number != 2 {
					continue
				}

				fields, err := decode(entry.data)
				if err != nil {
					return commandOutput{}, err
				}

				var cand candidate

				for _, field := range fields {
					switch field.number {
					case 1:
						cand.id = int32(field.value)
					case 4:
						cand.text = string(field.data)
					}
				}

				result.candidates = append(result.candidates, cand)
			}
		}
	}

	return result, nil
}
