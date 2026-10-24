package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hidori/NetKKC/internal/api"
	"github.com/hidori/NetKKC/internal/mozc"
	"github.com/labstack/echo/v4"
)

// requestError reports an invalid request. Details point at the invalid fields.
type requestError struct {
	message string
	details []api.ErrorDetail
}

func (e *requestError) Error() string {
	return e.message
}

func invalidRequest(message string) error {
	return &requestError{message: message}
}

func invalidField(field, reason string) error {
	return &requestError{
		message: "invalid request",
		details: []api.ErrorDetail{{Field: field, Reason: reason}},
	}
}

// errorResponses converts errors returned by handlers into HTTP errors whose
// message is an api.ErrorResponse. Echo's error handler writes them.
func errorResponses(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		err := next(ctx)
		if err == nil {
			return nil
		}

		status, response := errorResponse(err)
		if status >= http.StatusInternalServerError {
			slog.Error("request failed", "status", status, "error", fmt.Sprintf("%+v", err))
		}

		return echo.NewHTTPError(status, response).SetInternal(err)
	}
}

func errorResponse(err error) (int, api.ErrorResponse) {
	if reqErr, ok := errors.AsType[*requestError](err); ok {
		response := api.ErrorResponse{Message: reqErr.message}
		if len(reqErr.details) > 0 {
			response.Details = &reqErr.details
		}

		return http.StatusBadRequest, response
	}

	if httpErr, ok := errors.AsType[*echo.HTTPError](err); ok {
		return httpErr.Code, api.ErrorResponse{Message: fmt.Sprint(httpErr.Message)}
	}

	switch {
	case errors.Is(err, mozc.ErrBoundaries):
		return http.StatusUnprocessableEntity, api.ErrorResponse{Message: "Mozc cannot realize the requested boundaries"}
	case errors.Is(err, mozc.ErrInvalidResponse):
		return http.StatusBadGateway, api.ErrorResponse{Message: "Mozc returned an invalid response"}
	case errors.Is(err, mozc.ErrUnavailable):
		return http.StatusServiceUnavailable, api.ErrorResponse{Message: "Mozc is unavailable"}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return http.StatusGatewayTimeout, api.ErrorResponse{Message: "conversion timed out or was cancelled"}
	default:
		return http.StatusInternalServerError, api.ErrorResponse{Message: http.StatusText(http.StatusInternalServerError)}
	}
}
