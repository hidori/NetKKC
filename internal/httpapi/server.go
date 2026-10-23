package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	Convert(context.Context, string, []string) ([]mozc.Segment, error)
}

type Server struct {
	converter Converter
}

var _ api.StrictServerInterface = (*Server)(nil)

func New(converter Converter) *echo.Echo {
	app := echo.New()
	app.JSONSerializer = strictJSONSerializer{}
	app.Use(middleware.Recover(), middleware.BodyLimit("1M"))
	app.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			if ctx.Request().Method == http.MethodPost && ctx.Path() == "/convert" {
				kind, _, err := mime.ParseMediaType(ctx.Request().Header.Get(echo.HeaderContentType))
				if err != nil || kind != echo.MIMEApplicationJSON {
					return echo.NewHTTPError(http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				}
			}

			return next(ctx)
		}
	})
	api.RegisterHandlers(app, api.NewStrictHandler(&Server{converter: converter}, nil))

	return app
}

type strictJSONSerializer struct {
	echo.DefaultJSONSerializer
}

func (strictJSONSerializer) Deserialize(ctx echo.Context, target any) error {
	data, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		return errs.Wrap(err)
	}

	if !utf8.Valid(data) {
		return badRequest("request body must be valid UTF-8 JSON")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return badRequest("request body must contain one JSON object with valid field types")
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return badRequest("request body must contain only one JSON value")
	}

	return nil
}

func (s *Server) GetHealth(context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return api.GetHealth200TextResponse("OK"), nil
}

func (s *Server) Convert(ctx context.Context, request api.ConvertRequestObject) (api.ConvertResponseObject, error) {
	if request.Body == nil || request.Body.Reading == "" || !utf8.ValidString(request.Body.Reading) {
		return nil, badRequest("reading must be a non-empty UTF-8 string")
	}

	reading := request.Body.Reading

	var (
		boundaries    []string
		confirmations []*string
	)

	if request.Body.Segments != nil {
		for _, value := range *request.Body.Segments {
			segment, err := value.AsUnconfirmedSegmentRequest()
			if err != nil || segment.Reading == "" {
				return nil, badRequest("each segment must have a non-empty reading and valid field types")
			}
			// encoding/json treats null booleans as absent. The API accepts only
			// actual booleans when confirmed is present, not null.
			data, _ := value.MarshalJSON()

			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
				return nil, badRequest("each segment must be an object")
			}

			for _, key := range []string{"confirmed", "candidates"} {
				if bytes.Equal(fields[key], []byte("null")) {
					return nil, badRequest(key + " must not be null")
				}
			}

			boundaries = append(boundaries, segment.Reading)

			var confirmed *string

			if segment.Confirmed != nil && bool(*segment.Confirmed) {
				if segment.Candidates == nil || len(*segment.Candidates) != 1 || (*segment.Candidates)[0] == "" {
					return nil, badRequest("confirmed segments require exactly one non-empty candidate")
				}

				text := (*segment.Candidates)[0]
				confirmed = &text
			}

			confirmations = append(confirmations, confirmed)
		}
	}

	if len(boundaries) > 0 && strings.Join(boundaries, "") != reading {
		return nil, badRequest("segment readings must concatenate to the reading")
	}

	allConfirmed := len(confirmations) > 0
	for _, text := range confirmations {
		allConfirmed = allConfirmed && text != nil
	}

	var converted []mozc.Segment

	if allConfirmed {
		// A fully confirmed request needs no candidate generation or Mozc call.
		for _, part := range boundaries {
			converted = append(converted, mozc.Segment{Reading: part})
		}
	} else {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		var err error

		converted, err = s.converter.Convert(ctx, reading, boundaries)
		if err != nil {
			if errors.Is(err, mozc.ErrBoundaries) {
				return nil, echo.NewHTTPError(http.StatusUnprocessableEntity, "Mozc cannot realize the requested boundaries")
			}

			if ctx.Err() != nil {
				return nil, echo.NewHTTPError(http.StatusGatewayTimeout, "conversion timed out or was cancelled")
			}

			slog.Error("Mozc conversion failed", "error", fmt.Sprintf("%+v", err))

			return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "Mozc is unavailable")
		}
	}
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
			if len(segment.Candidates) == 0 {
				return nil, echo.NewHTTPError(http.StatusBadGateway, "Mozc returned no candidates")
			}

			err = response.Segments[i].FromUnconfirmedSegmentResponse(api.UnconfirmedSegmentResponse{
				Reading: segment.Reading, Confirmed: false, Candidates: segment.Candidates,
			})
		}

		if err != nil {
			return nil, errs.Wrap(err)
		}
	}

	return api.Convert200JSONResponse(response), nil
}

func badRequest(message string) error {
	return echo.NewHTTPError(http.StatusBadRequest, message)
}
