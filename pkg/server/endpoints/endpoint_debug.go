package endpoints

import (
	"github.com/kivra/gocc/pkg/limiter/limiter_manager"
	"github.com/labstack/echo/v4"
	"net/http"
)

func HandleDebugRequest(
	limiterManager *limiter_manager.LimiterManagerSet,
) echo.HandlerFunc {
	return func(c echo.Context) error {

		key := c.Param("key")

		if key != "" {
			snapshot := limiterManager.GetDebugSnapshot(key)
			if snapshot != nil {
				if snapshot.Found {
					return c.JSON(http.StatusOK, snapshot)
				} else {
					return c.String(http.StatusNotFound, "Key not found")
				}
			} else {
				return c.String(http.StatusInternalServerError, "Unable to get debug snapshot, check server logs")
			}
		} else {
			all := limiterManager.GetDebugSnapshotsAll()
			if all != nil {
				return c.JSON(http.StatusOK, all)
			} else {
				return c.String(http.StatusInternalServerError, "Unable to get debug snapshots, check server logs")
			}
		}
	}
}
