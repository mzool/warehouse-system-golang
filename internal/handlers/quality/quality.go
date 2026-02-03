package quality

import (
	"warehouse_system/internal/handlers"
)

type QualityHandler struct {
	h *handlers.Handler
}

func NewQualityHandler(h *handlers.Handler) *QualityHandler {
	return &QualityHandler{h: h}
}
