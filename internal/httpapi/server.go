package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hidori/NetKKC/internal/api"
	"github.com/hidori/NetKKC/internal/errs"
	"github.com/hidori/NetKKC/internal/mozc"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Converter interface {
	Convert(ctx context.Context, reading string, boundaries []string) ([]mozc.Segment, error)
}

type Server struct {
	converter Converter
}

var _ api.ServerInterface = (*Server)(nil)

func New(converter Converter) *echo.Echo {
	app := echo.New()
	app.JSONSerializer = strictJSONSerializer{}
	app.Use(errorResponses, middleware.Recover(), middleware.BodyLimit("1M"), requireJSON)
	api.RegisterHandlers(app, &Server{converter: converter})

	return app
}

func requireJSON(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		if ctx.Request().Method == http.MethodPost && ctx.Path() == "/convert" {
			kind, _, err := mime.ParseMediaType(ctx.Request().Header.Get(echo.HeaderContentType))
			if err != nil || kind != echo.MIMEApplicationJSON {
				return echo.NewHTTPError(http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			}
		}

		return next(ctx)
	}
}

type strictJSONSerializer struct {
	echo.DefaultJSONSerializer
}

func (strictJSONSerializer) Deserialize(ctx echo.Context, target any) error {
	data, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		// BodyLimit reports an oversized body as an HTTP error; keep its status.
		if httpErr, ok := errors.AsType[*echo.HTTPError](err); ok {
			return httpErr
		}

		return errs.Wrap(err)
	}

	if !utf8.Valid(data) {
		return invalidRequest("request body must be valid UTF-8 JSON")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return invalidRequest("request body must contain one JSON object with valid field types")
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalidRequest("request body must contain only one JSON value")
	}

	return nil
}

func (s *Server) GetHealth(ctx echo.Context) error {
	return errs.Wrap(ctx.Blob(http.StatusOK, "text/plain", []byte("OK")))
}

func (s *Server) Convert(ctx echo.Context) error {
	var body api.ConvertRequest
	if err := ctx.Bind(&body); err != nil {
		return errs.Wrap(err)
	}

	response, err := s.convert(ctx.Request().Context(), body)
	if err != nil {
		return err
	}

	return errs.Wrap(ctx.JSON(http.StatusOK, response))
}

// convert validates the request and returns the response for HTTP 200.
func (s *Server) convert(ctx context.Context, body api.ConvertRequest) (api.ConvertResponse, error) {
	if body.Reading == "" || !utf8.ValidString(body.Reading) {
		return api.ConvertResponse{}, invalidField("reading", "must be a non-empty UTF-8 string")
	}

	boundaries, confirmations, err := parseSegments(body.Segments)
	if err != nil {
		return api.ConvertResponse{}, err
	}

	if len(boundaries) > 0 && strings.Join(boundaries, "") != body.Reading {
		return api.ConvertResponse{}, invalidField("segments", "segment readings must concatenate to the reading")
	}

	converted, err := s.segments(ctx, body.Reading, boundaries, confirmations)
	if err != nil {
		return api.ConvertResponse{}, err
	}

	return buildResponse(converted, confirmations)
}

// parseSegments validates the requested segments. It returns their readings and,
// for each segment, the confirmed candidate or nil when it is unconfirmed.
func parseSegments(segments *[]api.SegmentRequest) ([]string, []*string, error) {
	if segments == nil {
		return nil, nil, nil
	}

	var (
		boundaries    []string
		confirmations []*string
	)

	for i, value := range *segments {
		reading, confirmed, err := parseSegment(i, value)
		if err != nil {
			return nil, nil, err
		}

		boundaries = append(boundaries, reading)
		confirmations = append(confirmations, confirmed)
	}

	return boundaries, confirmations, nil
}

func parseSegment(index int, value api.SegmentRequest) (string, *string, error) {
	field := fmt.Sprintf("segments[%d]", index)

	segment, err := value.AsUnconfirmedSegmentRequest()
	if err != nil {
		return "", nil, invalidField(field, "must have valid field types")
	}

	if segment.Reading == "" {
		return "", nil, invalidField(field+".reading", "must be a non-empty string")
	}
	// encoding/json treats null booleans as absent. The API accepts only
	// actual booleans when confirmed is present, not null.
	data, _ := value.MarshalJSON()

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return "", nil, invalidField(field, "must be an object")
	}

	for _, key := range []string{"confirmed", "candidates"} {
		if bytes.Equal(fields[key], []byte("null")) {
			return "", nil, invalidField(field+"."+key, "must not be null")
		}
	}

	if segment.Confirmed == nil || !bool(*segment.Confirmed) {
		return segment.Reading, nil, nil
	}

	if segment.Candidates == nil || len(*segment.Candidates) != 1 || (*segment.Candidates)[0] == "" {
		return "", nil, invalidField(field+".candidates", "confirmed segments require exactly one non-empty candidate")
	}

	text := (*segment.Candidates)[0]

	return segment.Reading, &text, nil
}

// segments returns the segments to respond with. It calls Mozc unless every
// segment is confirmed.
func (s *Server) segments(ctx context.Context, reading string, boundaries []string, confirmations []*string) ([]mozc.Segment, error) {
	if allConfirmed(confirmations) {
		// A fully confirmed request needs no candidate generation or Mozc call.
		converted := make([]mozc.Segment, 0, len(boundaries))
		for _, part := range boundaries {
			converted = append(converted, mozc.Segment{Reading: part})
		}

		return converted, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	converted, err := s.converter.Convert(ctx, reading, boundaries)
	if err != nil {
		return nil, errs.Wrap(err)
	}

	return converted, nil
}

func allConfirmed(confirmations []*string) bool {
	if len(confirmations) == 0 {
		return false
	}

	for _, text := range confirmations {
		if text == nil {
			return false
		}
	}

	return true
}

// buildResponse keeps the confirmed candidates and uses Mozc's candidates for
// the other segments.
func buildResponse(converted []mozc.Segment, confirmations []*string) (api.ConvertResponse, error) {
	// The response reading is Mozc's, which may differ from the request
	// (Mozc rewrites full-width ASCII to half-width, for example).
	response := api.ConvertResponse{Segments: make([]api.SegmentResponse, len(converted))}
	for i, segment := range converted {
		response.Reading += segment.Reading

		var err error
		if len(confirmations) > i && confirmations[i] != nil {
			err = response.Segments[i].FromConfirmedSegment(api.ConfirmedSegment{
				Reading: segment.Reading, Confirmed: api.True, Candidates: []string{*confirmations[i]},
			})
		} else {
			err = response.Segments[i].FromUnconfirmedSegmentResponse(api.UnconfirmedSegmentResponse{
				Reading: segment.Reading, Confirmed: false, Candidates: segment.Candidates,
			})
		}

		if err != nil {
			return api.ConvertResponse{}, errs.Wrap(err)
		}
	}

	return response, nil
}
