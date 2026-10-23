package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hidori/NetKKC/internal/api"
	"github.com/hidori/NetKKC/internal/mozc"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rivo/uniseg"
	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

type Converter interface {
	Convert(context.Context, string, []string) ([]mozc.Segment, error)
}

type Server struct {
	converter Converter
}

var _ api.StrictServerInterface = (*Server)(nil)

func New(converter Converter) *echo.Echo {
	e := echo.New()
	e.Use(middleware.Recover(), middleware.BodyLimit("1M"))
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Method == http.MethodPost && c.Path() == "/convert" {
				kind, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
				if err != nil || kind != echo.MIMEApplicationJSON {
					return echo.NewHTTPError(http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				}
			}
			return next(c)
		}
	})
	api.RegisterHandlers(e, api.NewStrictHandler(&Server{converter: converter}, nil))
	return e
}

func (s *Server) GetHealth(context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return api.GetHealth200TextResponse("OK"), nil
}

// Normalize only widens half-width katakana (U+FF66..U+FF9F), then applies NFC.
// Other compatibility characters and spacing dakuten/handakuten are preserved.
func Normalize(reading string) string {
	var widened strings.Builder
	for _, r := range reading {
		if r >= '\uff66' && r <= '\uff9f' {
			widened.WriteString(width.Widen.String(string(r)))
		} else {
			widened.WriteRune(r)
		}
	}
	return norm.NFC.String(widened.String())
}

func (s *Server) Convert(ctx context.Context, request api.ConvertRequestObject) (api.ConvertResponseObject, error) {
	if request.Body == nil || request.Body.Reading == "" || !utf8.ValidString(request.Body.Reading) {
		return nil, badRequest("reading must be a non-empty UTF-8 string")
	}
	reading := Normalize(request.Body.Reading)
	var boundaries []string
	var confirmations []*string
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
			boundaries = append(boundaries, Normalize(segment.Reading))
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
	if len(boundaries) > 0 {
		if strings.Join(boundaries, "") != reading {
			return nil, badRequest("segment readings must concatenate to the normalized reading")
		}
		if !validBoundaries(reading, boundaries) {
			return nil, badRequest("a segment boundary splits a grapheme cluster")
		}
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
			slog.Error("Mozc conversion failed", "error", err)
			return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "Mozc is unavailable")
		}
	}
	var returned []string
	for _, part := range converted {
		returned = append(returned, part.Reading)
	}
	if strings.Join(returned, "") != reading || len(returned) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Mozc returned an inconsistent reading")
	}
	if len(boundaries) > 0 && !slices.Equal(returned, boundaries) {
		return nil, echo.NewHTTPError(http.StatusUnprocessableEntity, "Mozc changed the requested boundaries")
	}
	if !validBoundaries(reading, returned) {
		return nil, echo.NewHTTPError(http.StatusUnprocessableEntity, "Mozc split a grapheme cluster")
	}
	response := api.ConvertResponse{Reading: reading, Segments: make([]api.SegmentResponse, len(converted))}
	for i, segment := range converted {
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
			return nil, err
		}
	}
	return api.Convert200JSONResponse(response), nil
}

func badRequest(message string) error {
	return echo.NewHTTPError(http.StatusBadRequest, message)
}

func validBoundaries(reading string, parts []string) bool {
	ends := map[int]bool{0: true}
	iterator := uniseg.NewGraphemes(reading)
	for iterator.Next() {
		_, end := iterator.Positions()
		ends[end] = true
	}
	position := 0
	for _, part := range parts {
		position += len(part)
		if part == "" || !ends[position] {
			return false
		}
	}
	return position == len(reading)
}
