package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hidori/NetKKC/internal/mozc"
	"github.com/labstack/echo/v4"
)

type converterFunc func(context.Context, string, []string) ([]mozc.Segment, error)

func (f converterFunc) Convert(ctx context.Context, reading string, boundaries []string) ([]mozc.Segment, error) {
	return f(ctx, reading, boundaries)
}

func request(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, req)
	return response
}

func TestNormalize(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"か\u3099", "が"}, {"は\u309a", "ぱ"}, {"が", "が"},
		{"ｶﾞﾊﾟ", "ガパ"}, {"か゛は゜", "か゛は゜"},
		{"①㍑ﬃＡ", "①㍑ﬃＡ"}, {"､｡", "､｡"},
	} {
		if got := Normalize(test.input); got != test.want {
			t.Errorf("Normalize(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestConfirmedMiddleSegment(t *testing.T) {
	calls := 0
	e := New(converterFunc(func(_ context.Context, reading string, boundaries []string) ([]mozc.Segment, error) {
		calls++
		if reading != "きょうはとうきょうにいく" || strings.Join(boundaries, "|") != "きょうは|とうきょうに|いく" {
			t.Fatalf("unexpected backend input: %s %v", reading, boundaries)
		}
		return []mozc.Segment{
			{Reading: "きょうは", Candidates: []string{"今日は"}},
			{Reading: "とうきょうに", Candidates: []string{"東京に", "トウキョウニ"}},
			{Reading: "いく", Candidates: []string{"行く", "逝く"}},
		}, nil
	}))
	response := request(e, http.MethodPost, "/convert", `{
		"reading":"きょうはとうきょうにいく",
		"segments":[
			{"reading":"きょうは","candidates":["使わない候補"]},
			{"reading":"とうきょうに","confirmed":true,"candidates":["クライアントが確定した文字列"]},
			{"reading":"いく","confirmed":false,"candidates":["使わない候補"]}
		]}`)
	if response.Code != http.StatusOK || calls != 1 {
		t.Fatalf("status %d, calls %d: %s", response.Code, calls, response.Body)
	}
	var result struct {
		Segments []struct {
			Confirmed  bool     `json:"confirmed"`
			Candidates []string `json:"candidates"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Segments) != 3 || result.Segments[0].Confirmed || !result.Segments[1].Confirmed || result.Segments[2].Confirmed {
		t.Fatalf("incorrect confirmation flags: %s", response.Body)
	}
	if strings.Join(result.Segments[1].Candidates, "|") != "クライアントが確定した文字列" ||
		strings.Join(result.Segments[0].Candidates, "|") != "今日は" ||
		strings.Join(result.Segments[2].Candidates, "|") != "行く|逝く" {
		t.Fatalf("incorrect candidates: %s", response.Body)
	}
}

func TestFullyConfirmedDoesNotCallMozc(t *testing.T) {
	e := New(converterFunc(func(context.Context, string, []string) ([]mozc.Segment, error) {
		t.Fatal("fully confirmed request must not call Mozc")
		return nil, nil
	}))
	response := request(e, http.MethodPost, "/convert", `{"reading":"か","segments":[{"reading":"か","confirmed":true,"candidates":["蚊"]}]}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"candidates":["蚊"]`) {
		t.Fatalf("%d: %s", response.Code, response.Body)
	}
}

func TestInvalidRequestsDoNotCallMozc(t *testing.T) {
	e := New(converterFunc(func(context.Context, string, []string) ([]mozc.Segment, error) {
		t.Fatal("invalid request reached Mozc")
		return nil, nil
	}))
	for _, body := range []string{
		``, `null`, `{}`, `{"reading":""}`, `{"reading":12}`, `{"reading":"か",`,
		`{"reading":"か","segments":[null]}`,
		`{"reading":"か","segments":[{"confirmed":false}]}`,
		`{"reading":"か","segments":[{"reading":"き"}]}`,
		`{"reading":"か","segments":[{"reading":"か","confirmed":null}]}`,
		`{"reading":"か","segments":[{"reading":"か","confirmed":"true"}]}`,
		`{"reading":"か","segments":[{"reading":"か","confirmed":true}]}`,
		`{"reading":"か","segments":[{"reading":"か","confirmed":true,"candidates":[]}]}`,
		`{"reading":"か","segments":[{"reading":"か","confirmed":true,"candidates":["蚊","科"]}]}`,
		`{"reading":"か","segments":[{"reading":"か","confirmed":true,"candidates":[""]}]}`,
		`{"reading":"か","segments":[{"reading":"か","candidates":null}]}`,
		`{"reading":"か","segments":[{"reading":"か","candidates":[1]}]}`,
		`{"reading":"か\u3099","segments":[{"reading":"か"},{"reading":"\u3099"}]}`,
		`{"reading":"葛\ufe00","segments":[{"reading":"葛"},{"reading":"\ufe00"}]}`,
		`{"reading":"👩‍💻","segments":[{"reading":"👩"},{"reading":"‍💻"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			response := request(e, http.MethodPost, "/convert", body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d: %s", response.Code, response.Body)
			}
		})
	}
}

func TestNormalizeBeforeConversion(t *testing.T) {
	for _, suffix := range []string{``, `,"segments":[]`, `,"segments":[{"reading":"か\u3099ｶﾞ"}]`} {
		e := New(converterFunc(func(_ context.Context, reading string, boundaries []string) ([]mozc.Segment, error) {
			if reading != "がガ" {
				t.Fatalf("not normalized: %q", reading)
			}
			return []mozc.Segment{{Reading: reading, Candidates: []string{"がガ"}}}, nil
		}))
		response := request(e, http.MethodPost, "/convert", `{"reading":"か\u3099ｶﾞ"`+suffix+`}`)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reading":"がガ"`) {
			t.Fatalf("%d: %s", response.Code, response.Body)
		}
	}
}

func TestBackendErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{mozc.ErrBoundaries, http.StatusUnprocessableEntity},
		{errors.New("connection refused"), http.StatusServiceUnavailable},
	} {
		e := New(converterFunc(func(context.Context, string, []string) ([]mozc.Segment, error) { return nil, test.err }))
		response := request(e, http.MethodPost, "/convert", `{"reading":"か"}`)
		if response.Code != test.status {
			t.Fatalf("want %d, got %d: %s", test.status, response.Code, response.Body)
		}
	}
}

func TestHealthAndRemovedGreeting(t *testing.T) {
	e := New(nil)
	response := request(e, http.MethodGet, "/healsh", "")
	if response.Code != http.StatusOK || response.Body.String() != "OK" || response.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("incorrect health response: %+v", response)
	}
	if response := request(e, http.MethodGet, "/", ""); response.Code != http.StatusNotFound {
		t.Fatalf("root endpoint must be removed, got %d", response.Code)
	}
}

func TestHTTPMozcIntegration(t *testing.T) {
	profile := os.Getenv("NETKKC_TEST_MOZC_PROFILE")
	if profile == "" {
		t.Skip("set NETKKC_TEST_MOZC_PROFILE to a dedicated running Mozc profile")
	}
	client := &mozc.Client{Profile: profile}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	e := New(mozc.NewConverter(client))
	response := request(e, http.MethodPost, "/convert", `{"reading":"こんにちは、きょうはいいお天気ですね"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("%d: %s", response.Code, response.Body)
	}
	var converted struct {
		Reading  string `json:"reading"`
		Segments []struct {
			Reading    string   `json:"reading"`
			Candidates []string `json:"candidates"`
			Confirmed  bool     `json:"confirmed"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &converted); err != nil {
		t.Fatal(err)
	}
	if len(converted.Segments) == 0 || converted.Reading != "こんにちは、きょうはいいお天気ですね" {
		t.Fatalf("unexpected response: %s", response.Body)
	}
	var joined strings.Builder
	for i, segment := range converted.Segments {
		if len(segment.Candidates) == 0 || segment.Confirmed {
			t.Fatalf("missing candidates or unexpected confirmation: %s", response.Body)
		}
		joined.WriteString(segment.Reading)
		if i == 2 {
			segment.Confirmed = true
			segment.Candidates = segment.Candidates[:1]
			converted.Segments[i] = segment
		}
	}
	if joined.String() != converted.Reading {
		t.Fatal("segment readings do not preserve input")
	}
	body, err := json.Marshal(converted)
	if err != nil {
		t.Fatal(err)
	}
	if response := request(e, http.MethodPost, "/convert", string(body)); response.Code != http.StatusOK {
		t.Fatalf("reconversion failed: %d: %s", response.Code, response.Body)
	}
}
