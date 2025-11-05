package handlers

import (
	"context"

	"encore.app/chat/domain"
	"encore.app/chat/internal"
)

type ThreadsHandler struct {
	threadRepo  domain.ThreadRepository
	messageRepo domain.MessageRepository
	logger      internal.ServiceLogger
	authHelper  *internal.AuthHelper
	validator   *domain.Validator
}

func NewThreadsHandler(
	threadRepo domain.ThreadRepository,
	messageRepo domain.MessageRepository,
	logger internal.ServiceLogger,
	authHelper *internal.AuthHelper,
	validator *domain.Validator,
) *ThreadsHandler {
	return &ThreadsHandler{
		threadRepo:  threadRepo,
		messageRepo: messageRepo,
		logger:      logger,
		authHelper:  authHelper,
		validator:   validator,
	}
}

// ListThreads retrieves threads for the authenticated user
func (h *ThreadsHandler) ListThreads(ctx context.Context, params *ListThreadsRequest) (*ThreadsResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "list_threads")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil, err
	}

	// Normalize pagination parameters
	offset, limit, err := internal.HandlePaginationParams(params.Offset, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Offset = offset
	params.Limit = limit

	// Get threads for user
	threads, err := h.threadRepo.GetByParticipant(ctx, userCtx.ID, params.Limit, params.Offset)
	if err != nil {
		return nil, internal.HandleRepositoryError(err)
	}

	responses := make([]*ThreadResponse, len(threads))
	for i, thread := range threads {
		responses[i] = ThreadToResponse(thread)
	}

	return &ThreadsResponse{
		Threads: responses,
		Total:   len(responses),
	}, nil
}
