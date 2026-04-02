package extractors

import (
	"github.com/Space-DF/transformer-service/internal/models"
)

func init() {
	// Register LNS handlers (singletons)
	models.RegisterLNSHandler(models.LNSTypeChirpStack, &ChirpStackHandler{})
	models.RegisterLNSHandler(models.LNSTypeTTN, &TTNHandler{})
	models.RegisterLNSHandler(models.LNSTypeHelium, &HeliumHandler{})
}
