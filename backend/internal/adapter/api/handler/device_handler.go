package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// DeviceHandler handles device token registration for push notifications.
type DeviceHandler struct {
	repo *repository.DeviceTokenRepository
	log  *logger.Logger
}

// NewDeviceHandler creates a new DeviceHandler.
func NewDeviceHandler(repo *repository.DeviceTokenRepository, log *logger.Logger) *DeviceHandler {
	return &DeviceHandler{repo: repo, log: log}
}

type registerTokenRequest struct {
	Token    string `json:"token"    binding:"required"`
	Platform string `json:"platform"`
}

// RegisterToken stores the caller's device push token.
// POST /api/v1/devices/token  (requires JWT auth)
func (h *DeviceHandler) RegisterToken(c *gin.Context) {
	userID := c.GetUint("userID")

	var req registerTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	platform := req.Platform
	if platform == "" {
		platform = "ios"
	}

	dt := &model.DeviceToken{
		UserID:   userID,
		Token:    req.Token,
		Platform: platform,
	}

	if err := h.repo.Save(dt); err != nil {
		h.log.Errorf("Failed to save device token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register device token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "device token registered"})
}
