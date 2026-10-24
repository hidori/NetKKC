package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hidori/NetKKC/internal/api"
	"github.com/hidori/NetKKC/internal/mozc"
	"github.com/labstack/echo/v4"
)

type converterFunc func(context.Context, string, []string) ([]mozc.Segment, error)

func (f converterFunc) Convert(ctx context.Context, reading string, boundaries []string) ([]mozc.Segment, error) {
	return f(ctx, reading, boundaries)
}

type convertResponse struct {
	Segments []struct {
		Reading    string   `json:"reading"`
		Candidates []string `json:"candidates"`
		Confirmed  bool     `json:"confirmed"`
	} `json:"segments"`
}

func request(t *testing.T, e *echo.Echo, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)

	response := httptest.NewRecorder()
	e.ServeHTTP(response, req)

	return response
}

func mustNotCallMozc(t *testing.T) converterFunc {
	t.Helper()

	return func(context.Context, string, []string) ([]mozc.Segment, error) {
		t.Fatal("request must not call Mozc")

		return nil, nil
	}
}

func TestConvert(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		path           string
		body           string
		wantReading    string
		wantBoundaries []string
		segments       []mozc.Segment
		want           string
	}{
		{
			name: "confirmed middle segment",
			path: "/reconvert",
			body: `{
				"segments":[
					{"reading":"きょうは","candidates":["使わない候補"]},
					{"reading":"とうきょうに","confirmed":true,"candidates":["クライアントが確定した文字列"]},
					{"reading":"いく","confirmed":false,"candidates":["使わない候補"]}
				]}`,
			wantReading:    "きょうはとうきょうにいく",
			wantBoundaries: []string{"きょうは", "とうきょうに", "いく"},
			segments: []mozc.Segment{
				{Reading: "きょうは", Candidates: []string{"今日は"}},
				{Reading: "とうきょうに", Candidates: []string{"東京に", "トウキョウニ"}},
				{Reading: "いく", Candidates: []string{"行く", "逝く"}},
			},
			want: `{
				"segments":[
					{"reading":"きょうは","confirmed":false,"candidates":["今日は"]},
					{"reading":"とうきょうに","confirmed":true,"candidates":["クライアントが確定した文字列"]},
					{"reading":"いく","confirmed":false,"candidates":["行く","逝く"]}
				]}`,
		},
		{
			name: "fully confirmed does not call Mozc",
			path: "/reconvert",
			body: `{"segments":[{"reading":"か","confirmed":true,"candidates":["蚊"]}]}`,
			want: `{"segments":[{"reading":"か","confirmed":true,"candidates":["蚊"]}]}`,
		},
		{
			name:        "response uses Mozc reading",
			path:        "/convert",
			body:        `{"reading":"ＡＢＣのてすと","segments":[{"reading":"無視される"}]}`,
			wantReading: "ＡＢＣのてすと",
			segments: []mozc.Segment{
				{Reading: "ABCの", Candidates: []string{"ABCの"}},
				{Reading: "てすと", Candidates: []string{"テスト"}},
			},
			want: `{
				"segments":[
					{"reading":"ABCの","confirmed":false,"candidates":["ABCの"]},
					{"reading":"てすと","confirmed":false,"candidates":["テスト"]}
				]}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			converter := mustNotCallMozc(t)
			if test.segments != nil {
				converter = func(_ context.Context, reading string, boundaries []string) ([]mozc.Segment, error) {
					if reading != test.wantReading || !reflect.DeepEqual(boundaries, test.wantBoundaries) {
						t.Fatalf("unexpected backend input: %q %v", reading, boundaries)
					}

					return test.segments, nil
				}
			}

			response := request(t, New(converter), http.MethodPost, test.path, "application/json", test.body)
			if response.Code != http.StatusOK {
				t.Fatalf("%d: %s", response.Code, response.Body)
			}

			var got, want convertResponse
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}

			if err := json.Unmarshal([]byte(test.want), &want); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("incorrect response: %s", response.Body)
			}
		})
	}
}

func TestInvalidRequestsDoNotCallMozc(t *testing.T) {
	t.Parallel()

	longReading := strings.Repeat("a", 4097)

	for _, test := range []struct{ path, body string }{
		{"/convert", ``}, {"/convert", `null`}, {"/convert", `{}`}, {"/convert", `{"reading":""}`},
		{"/convert", `{"reading":12}`}, {"/convert", `{"reading":"か",`},
		{"/convert", `{"reading":"か"}{"reading":"き"}`}, {"/convert", `{"reading":"か"} trailing text`},
		{"/convert", "{\"reading\":\"\xff\"}"},
		{"/convert", `{"reading":"` + longReading + `"}`},
		{"/reconvert", `{}`}, {"/reconvert", `{"segments":[]}`},
		{"/reconvert", `{"segments":[null]}`},
		{"/reconvert", `{"segments":[{"confirmed":false}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","confirmed":null}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","confirmed":"true"}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","confirmed":true}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","confirmed":true,"candidates":[]}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","confirmed":true,"candidates":["蚊","科"]}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","confirmed":true,"candidates":[""]}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","candidates":null}]}`},
		{"/reconvert", `{"segments":[{"reading":"か","candidates":[1]}]}`},
		{"/reconvert", `{"segments":[{"reading":"` + longReading[:4000] + `"},{"reading":"` + longReading[:97] + `"}]}`},
	} {
		t.Run(test.path+" "+test.body, func(t *testing.T) {
			t.Parallel()

			response := request(t, New(mustNotCallMozc(t)), http.MethodPost, test.path, "application/json", test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d: %s", response.Code, response.Body)
			}
		})
	}
}

func TestInvalidRequestDetails(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		path  string
		body  string
		field string
	}{
		{"/convert", `{"reading":""}`, "reading"},
		{"/convert", `{"reading":"` + strings.Repeat("a", 4097) + `"}`, "reading"},
		{"/convert", `{"reading":"か","max_candidates":0}`, "max_candidates"},
		{"/convert", `{"reading":"か","max_candidates":1001}`, "max_candidates"},
		{"/reconvert", `{}`, "segments"},
		{"/reconvert", `{"segments":[{"reading":"か","confirmed":null}]}`, "segments[0].confirmed"},
		{"/reconvert", `{"segments":[{"reading":"か"},{"reading":"き","confirmed":true,"candidates":[]}]}`, "segments[1].candidates"},
	} {
		t.Run(test.path+" "+test.body, func(t *testing.T) {
			t.Parallel()

			response := request(t, New(mustNotCallMozc(t)), http.MethodPost, test.path, "application/json", test.body)

			var got api.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}

			if response.Code != http.StatusBadRequest || got.Details == nil || len(*got.Details) != 1 || (*got.Details)[0].Field != test.field {
				t.Fatalf("want 400 with details for %s, got %d: %s", test.field, response.Code, response.Body)
			}
		})
	}
}

func TestCandidateLimit(t *testing.T) {
	t.Parallel()

	candidates := make([]string, 25)
	for i := range candidates {
		candidates[i] = strconv.Itoa(i)
	}

	e := New(converterFunc(func(context.Context, string, []string) ([]mozc.Segment, error) {
		return []mozc.Segment{{Reading: "か", Candidates: candidates}}, nil
	}))

	for _, test := range []struct {
		body string
		want int
	}{
		{`{"reading":"か"}`, 20},
		{`{"reading":"か","max_candidates":3}`, 3},
		{`{"reading":"か","max_candidates":1000}`, 25},
	} {
		t.Run(test.body, func(t *testing.T) {
			t.Parallel()

			response := request(t, e, http.MethodPost, "/convert", "application/json", test.body)

			var got struct {
				Segments []struct {
					Candidates []string `json:"candidates"`
				} `json:"segments"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}

			if response.Code != http.StatusOK || len(got.Segments) != 1 || !reflect.DeepEqual(got.Segments[0].Candidates, candidates[:test.want]) {
				t.Fatalf("want the first %d candidates, got %d: %s", test.want, response.Code, response.Body)
			}
		})
	}
}

func TestBackendErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"boundaries", mozc.ErrBoundaries, http.StatusUnprocessableEntity},
		{"invalid response", fmt.Errorf("%w: no segments", mozc.ErrInvalidResponse), http.StatusBadGateway},
		{"unavailable", fmt.Errorf("%w: connect", mozc.ErrUnavailable), http.StatusServiceUnavailable},
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"unexpected", errors.New("unexpected"), http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			e := New(converterFunc(func(context.Context, string, []string) ([]mozc.Segment, error) { return nil, test.err }))

			response := request(t, e, http.MethodPost, "/convert", "application/json", `{"reading":"か"}`)
			if response.Code != test.status {
				t.Fatalf("want %d, got %d: %s", test.status, response.Code, response.Body)
			}
		})
	}
}

func TestGetEndpoints(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name            string
		path            string
		wantStatus      int
		wantBody        string
		wantContentType string
	}{
		{"health", "/healsh", http.StatusOK, "OK", "text/plain"},
		{"removed greeting", "/", http.StatusNotFound, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := request(t, New(nil), http.MethodGet, test.path, "application/json", "")
			if response.Code != test.wantStatus {
				t.Fatalf("want %d, got %d: %s", test.wantStatus, response.Code, response.Body)
			}

			if test.wantBody != "" && (response.Body.String() != test.wantBody || response.Header().Get("Content-Type") != test.wantContentType) {
				t.Fatalf("incorrect response: %+v", response)
			}
		})
	}
}

func TestTransportErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{"content type", "text/plain", `{"reading":"か"}`, http.StatusUnsupportedMediaType},
		{"body limit", "application/json", `{"reading":"` + strings.Repeat("a", 1<<20) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := request(t, New(mustNotCallMozc(t)), http.MethodPost, "/convert", test.contentType, test.body)
			if response.Code != test.wantStatus {
				t.Fatalf("want %d, got %d: %s", test.wantStatus, response.Code, response.Body)
			}
		})
	}
}

//nolint:paralleltest // Uses the shared real Mozc server.
func TestHTTPMozcIntegration(t *testing.T) {
	profile := os.Getenv("NETKKC_TEST_MOZC_PROFILE")
	if profile == "" {
		t.Skip("set NETKKC_TEST_MOZC_PROFILE to a dedicated running Mozc profile")
	}

	client := &mozc.Client{Profile: profile}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := client.Configure(ctx); err != nil {
		t.Fatal(err)
	}

	e := New(mozc.NewConverter(client))

	converted := post(t, e, "/convert", `{"reading":"こんにちは、きょうはいいお天気ですね"}`)
	checkInitialConversion(t, converted, "こんにちは、きょうはいいお天気ですね")

	converted.Segments[2].Confirmed = true
	converted.Segments[2].Candidates = converted.Segments[2].Candidates[:1]
	selected := converted.Segments[2].Candidates[0]
	converted.Segments[0].Candidates = []string{"再生成で捨てられる候補"}

	body, err := json.Marshal(map[string]any{"segments": converted.Segments})
	if err != nil {
		t.Fatal(err)
	}

	checkReconversion(t, post(t, e, "/reconvert", string(body)), selected, "再生成で捨てられる候補")
}

func post(t *testing.T, e *echo.Echo, path, body string) convertResponse {
	t.Helper()

	response := request(t, e, http.MethodPost, path, "application/json", body)
	if response.Code != http.StatusOK {
		t.Fatalf("%d: %s", response.Code, response.Body)
	}

	var converted convertResponse
	if err := json.Unmarshal(response.Body.Bytes(), &converted); err != nil {
		t.Fatal(err)
	}

	return converted
}

func checkInitialConversion(t *testing.T, converted convertResponse, reading string) {
	t.Helper()

	if len(converted.Segments) < 3 {
		t.Fatalf("unexpected response: %+v", converted)
	}

	var joined strings.Builder

	for _, segment := range converted.Segments {
		if len(segment.Candidates) == 0 || segment.Confirmed {
			t.Fatalf("missing candidates or unexpected confirmation: %+v", converted)
		}

		joined.WriteString(segment.Reading)
	}

	if joined.String() != reading {
		t.Fatal("segment readings do not preserve input")
	}
}

func checkReconversion(t *testing.T, converted convertResponse, selected, discarded string) {
	t.Helper()

	if !converted.Segments[2].Confirmed || len(converted.Segments[2].Candidates) != 1 || converted.Segments[2].Candidates[0] != selected {
		t.Fatalf("confirmed candidate changed: %+v", converted)
	}

	for _, value := range converted.Segments[0].Candidates {
		if value == discarded {
			t.Fatal("unconfirmed client candidates were used")
		}
	}
}
