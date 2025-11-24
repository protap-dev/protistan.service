package handlers

import (
	"context"

	"encore.app/artisans/domain"
	"encore.app/artisans/internal"
)

// CategoriesResponse represents the response for getting all categories
type CategoriesResponse struct {
	Categories []domain.ServiceCategory `json:"categories"`
}

// CategoriesHandler handles service category operations
type CategoriesHandler struct {
	ratesRepo domain.RatesRepository
	logger    internal.Logger
}

// NewCategoriesHandler creates a new categories handler
func NewCategoriesHandler(ratesRepo domain.RatesRepository, logger internal.Logger) *CategoriesHandler {
	return &CategoriesHandler{
		ratesRepo: ratesRepo,
		logger:    logger,
	}
}

// GetAllCategories handles GET /artisans/categories
func (h *CategoriesHandler) GetAllCategories(ctx context.Context) (*CategoriesResponse, error) {
	h.logger.LogUserAction(ctx, "get_all_categories", "public_request")

	categories, err := h.ratesRepo.GetAllCategories(ctx)
	if err != nil {
		h.logger.LogError(ctx, "get_categories_failed", err)
		return nil, internal.ErrDatabaseError
	}

	h.logger.LogUserAction(ctx, "get_categories_success", "success")
	return &CategoriesResponse{
		Categories: categories,
	}, nil
}