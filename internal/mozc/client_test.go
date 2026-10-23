package mozc

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestDecodeOutput(t *testing.T) {
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
	data = message(data, 100, []byte("unknown field"))
	out, err := decodeOutput(data)
	if err != nil {
		t.Fatal(err)
	}
	if out.id != 42 || len(out.segments) != 1 || out.segments[0].reading != "きょうは" || focused(out) != 0 {
		t.Fatalf("incorrect session or segment: %+v", out)
	}
	if len(out.candidates) != 1 || out.candidates[0].id != -1 || out.candidates[0].text != "今日は" {
		t.Fatalf("incorrect candidate: %+v", out.candidates)
	}
}

func TestDecodeMalformedOutput(t *testing.T) {
	for _, data := range [][]byte{{0xff}, {0x2a, 0x05, 0x01}, {0x2a, 0x01, 0x13}, number(nil, 11, 1)} {
		if _, err := decodeOutput(data); err == nil {
			t.Errorf("expected malformed output to fail: %x", data)
		}
	}
}

func TestMozcIntegration(t *testing.T) {
	profile := os.Getenv("NETKKC_TEST_MOZC_PROFILE")
	if profile == "" {
		t.Skip("set NETKKC_TEST_MOZC_PROFILE to a dedicated running Mozc profile")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	} {
		t.Run(test.name, func(t *testing.T) {
			segments, err := converter.Convert(ctx, test.reading, test.boundaries)
			if err != nil {
				t.Fatal(err)
			}
			var readings []string
			for _, segment := range segments {
				readings = append(readings, segment.Reading)
				if len(segment.Candidates) == 0 {
					t.Fatal("missing candidates")
				}
			}
			if strings.Join(readings, "") != test.reading {
				t.Fatalf("reading changed: %v", readings)
			}
			if test.boundaries != nil && strings.Join(readings, "|") != strings.Join(test.boundaries, "|") {
				t.Fatalf("boundaries changed: %v", readings)
			}
		})
	}
}
