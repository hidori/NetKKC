package main

import (
	"net/http"

	echo "github.com/labstack/echo/v4"
)

func main() {
	e := echo.New()
	e.GET("/healsh", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})
	e.Logger.Fatal(e.Start(":8080"))
}
