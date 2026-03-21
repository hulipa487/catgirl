package http

import (
	"net/http"

	"github.com/hulipa487/catgirl/internal/services/telegram"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramHandler struct {
	svc *telegram.TelegramService
}

func NewTelegramHandler(svc *telegram.TelegramService) *TelegramHandler {
	return &TelegramHandler{svc: svc}
}

func (h *TelegramHandler) HandleWebhook(c *gin.Context) {
	// Optional check: you can verify c.Param("token") == botToken

	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.HandleUpdate(&update); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
