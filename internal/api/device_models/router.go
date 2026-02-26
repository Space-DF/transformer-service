package device_models

import (
	"github.com/Space-DF/transformer-service/internal/services"
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Group, dps *services.DeviceProfileService) {
	group := e.Group("/device-models")
	group.GET("/", getDeviceModels(dps))
}
