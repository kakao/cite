package controller

import (
	"net/http"

	"github.com/labstack/echo"
)

func GetWatchcenterGroupID(c echo.Context) error {
	return c.Render(http.StatusOK, "notification/watchcenter_popup", map[string]interface{}{})
}
