package device_models

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Space-DF/transformer-service/internal/api/common"
	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/services"
	"github.com/labstack/echo/v4"
)

func getDeviceModels(dps *services.DeviceProfileService) echo.HandlerFunc {
	return func(c echo.Context) error {
		deviceModels := dps.GetAllDeviceModels()

		// Filter by search query (limit length to prevent abuse)
		search := strings.TrimSpace(strings.ToLower(c.QueryParam("search")))
		const maxSearchLen = 100
		if len(search) > maxSearchLen {
			search = search[:maxSearchLen]
		}
		if search != "" {
			filtered := make([]models.DeviceModel, 0)
			for _, dm := range deviceModels {
				if strings.Contains(strings.ToLower(dm.Name), search) ||
					strings.Contains(strings.ToLower(dm.DeviceType), search) {
					filtered = append(filtered, dm)
				}
			}
			deviceModels = filtered
		}

		// Always sort by name ascending
		sort.Slice(deviceModels, func(i, j int) bool {
			return deviceModels[i].DeviceType < deviceModels[j].DeviceType
		})

		total := len(deviceModels)
		p := common.ParsePagination(c, 10)
		start, end := common.SlicePage(total, p)

		extra := url.Values{}
		if search != "" {
			extra.Set("search", search)
		}
		next, previous := common.Paginate(total, p, common.BuildBaseURL(c), extra)

		return c.JSON(http.StatusOK, common.PaginatedResponse{
			Count:    total,
			Next:     next,
			Previous: previous,
			Results:  deviceModels[start:end],
		})
	}
}
