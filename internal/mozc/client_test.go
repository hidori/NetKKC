package mozc

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func encodedOutput() []byte {
	segment := number(nil, 3, 2)
	segment = message(segment, 4, []byte("今日は"))
	segment = message(segment, 6, []byte("きょうは"))
	preedit := protowire.AppendTag(nil, 2, protowire.StartGroupType)
	preedit = append(preedit, segment...)
	preedit = protowire.AppendTag(preedit, 2, protowire.EndGroupType)
	word := number(nil, 1, ^uint64(0))
	word = message(word, 4, []byte("今日は"))
	data := message(number(nil, 1, 42), 5, preedit)
	data = message(data, 14, message(nil, 2, word))

	return message(data, 100, []byte("unknown field"))
}

func TestDecodeOutput(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		data    []byte
		want    commandOutput
		wantErr bool
	}{
		{
			name: "valid",
			data: encodedOutput(),
			want: commandOutput{
				id:         42,
				segments:   []preeditSegment{{reading: "きょうは", text: "今日は", focused: true}},
				candidates: []candidate{{id: -1, text: "今日は"}},
			},
		},
		{name: "truncated tag", data: []byte{0xff}, wantErr: true},
		{name: "truncated preedit", data: []byte{0x2a, 0x05, 0x01}, wantErr: true},
		{name: "malformed preedit", data: []byte{0x2a, 0x01, 0x13}, wantErr: true},
		{name: "session error", data: number(nil, 11, 1), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			out, err := decodeOutput(test.data)
			if (err != nil) != test.wantErr {
				t.Fatalf("want error %v, got %v", test.wantErr, err)
			}

			if !test.wantErr && !reflect.DeepEqual(out, test.want) {
				t.Fatalf("incorrect output: %+v", out)
			}
		})
	}
}

//nolint:paralleltest // Uses the shared real Mozc server.
func TestMozcIntegration(t *testing.T) {
	profile := os.Getenv("NETKKC_TEST_MOZC_PROFILE")
	if profile == "" {
		t.Skip("set NETKKC_TEST_MOZC_PROFILE to a dedicated running Mozc profile")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	client := &Client{Profile: profile}
	if err := client.Configure(ctx); err != nil {
		t.Fatal(err)
	}

	converter := NewConverter(client)

	for _, test := range []struct {
		name       string
		reading    string
		boundaries []string
	}{
		{"automatic", "こんにちは、きょうはいいお天気ですね", nil},
		{"expanded", "きょうはとうきょうにいく", []string{"きょうはと", "うきょうに", "いく"}},
		{"shrunk", "きょうはとうきょうにいく", []string{"きょう", "はとうきょうに", "いく"}},
		{"single", "きょうはとうきょうにいく", []string{"きょうはとうきょうにいく"}},
		{"rewritten", "ＡＢＣのてすと", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			segments, err := converter.Convert(ctx, test.reading, test.boundaries)
			if err != nil {
				t.Fatal(err)
			}

			readings := make([]string, 0, len(segments))
			for _, segment := range segments {
				readings = append(readings, segment.Reading)
				if len(segment.Candidates) == 0 {
					t.Fatal("missing candidates")
				}
			}

			if want := strings.Replace(test.reading, "ＡＢＣ", "ABC", 1); strings.Join(readings, "") != want {
				t.Fatalf("reading changed: %v", readings)
			}

			if test.boundaries != nil && strings.Join(readings, "|") != strings.Join(test.boundaries, "|") {
				t.Fatalf("boundaries changed: %v", readings)
			}
		})
	}
}
