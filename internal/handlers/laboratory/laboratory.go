package laboratory

import (
	"warehouse_system/internal/handlers"
)

// LaboratoryHandler handles all laboratory-related requests
type LaboratoryHandler struct {
	h *handlers.Handler
}

func NewLaboratoryHandler(h *handlers.Handler) *LaboratoryHandler {
	return &LaboratoryHandler{h: h}
}
