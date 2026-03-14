package customer

import (
	"net/http"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type UpdateProfileRequest struct {
	Name  *string `json:"name" validate:"omitempty,max=100"`
	Email *string `json:"email" validate:"omitempty,email,max=254"`
	Phone *string `json:"phone" validate:"omitempty,max=20"`
}

type ProfileResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	Phone     *string `json:"phone,omitempty"`
	UpdatedAt string  `json:"updated_at"`
}

func (h *CustomerHandler) UpdateProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		respondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req UpdateProfileRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	sanitizeRequest(&req)

	if req.Name == nil && req.Email == nil && req.Phone == nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "at least one field must be provided")
		return
	}

	params := services.ProfileUpdateParams{
		Name:  req.Name,
		Email: req.Email,
		Phone: req.Phone,
	}

	customer, err := h.customerService.UpdateProfile(c.Request.Context(), userID, c.ClientIP(), params)
	if err != nil {
		h.logger.Warn("profile update failed",
			"user_id", userID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if validationErr, ok := err.(*sharederrors.ValidationError); ok {
			respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", validationErr.Error())
			return
		}

		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NOT_FOUND", "customer not found")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "UPDATE_FAILED", "failed to update profile")
		return
	}

	resp := ProfileResponse{
		ID:        customer.ID,
		Name:      customer.Name,
		Email:     customer.Email,
		Phone:     customer.Phone,
		UpdatedAt: customer.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	h.logger.Info("profile updated",
		"user_id", userID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

func (h *CustomerHandler) GetProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		respondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	customer, err := h.customerRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NOT_FOUND", "customer not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "FETCH_FAILED", "failed to get profile")
		return
	}

	resp := ProfileResponse{
		ID:        customer.ID,
		Name:      customer.Name,
		Email:     customer.Email,
		Phone:     customer.Phone,
		UpdatedAt: customer.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

func sanitizeRequest(req *UpdateProfileRequest) {
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
	}
	if req.Email != nil {
		trimmed := strings.TrimSpace(*req.Email)
		req.Email = &trimmed
	}
	if req.Phone != nil {
		trimmed := strings.TrimSpace(*req.Phone)
		req.Phone = &trimmed
	}
}
