package endpoints

import (
	"github.com/labstack/echo/v4"
)

func HandleHealthRequest(
	c echo.Context,
) error {
	return c.NoContent(200)
}
