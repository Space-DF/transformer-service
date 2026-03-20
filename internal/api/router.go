package api

import (
	"github.com/Space-DF/transformer-service/internal/api/device_models"
	"github.com/Space-DF/transformer-service/internal/services"
	"github.com/labstack/echo/v4"
)

func Setup(e *echo.Group, dps *services.DeviceProfileService) {
	device_models.RegisterRoutes(e, dps)
}
