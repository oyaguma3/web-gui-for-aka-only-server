package handlers

import (
	"log/slog"
	"net/http"

	"aka-webgui/internal/client"
	"aka-webgui/internal/server/validators"

	"github.com/gin-gonic/gin"
)

type SubscriberHandler struct {
	Client *client.Client
}

func NewSubscriberHandler(c *client.Client) *SubscriberHandler {
	return &SubscriberHandler{Client: c}
}

func (h *SubscriberHandler) Dashboard(c *gin.Context) {
	count, err := h.Client.GetSubscriberCount()
	if err != nil {
		slog.Error("Failed to get subscriber count", "error", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Error": "Failed to connect to API server"})
		return
	}

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Title":      "Dashboard - AKA-Only-Server Web GUI",
		"TotalCount": count,
		"ShowList":   true, // Initially show list, or handle >100 logic here if desired
	})
}

func (h *SubscriberHandler) ListSubscribers(c *gin.Context) {
	subs, err := h.Client.GetSubscribers()
	if err != nil {
		slog.Error("Failed to get subscribers", "error", err)
		c.String(http.StatusInternalServerError, "Failed to fetch subscribers")
		return
	}

	c.HTML(http.StatusOK, "subscriber_list.html", gin.H{
		"Subscribers": subs,
	})
}

func (h *SubscriberHandler) CreateSubscriber(c *gin.Context) {
	var sub client.Subscriber
	if err := c.ShouldBindJSON(&sub); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if !validators.ValidateIMSI(sub.IMSI) || !validators.ValidateKey(sub.Ki) || !validators.ValidateOPC(sub.Opc) || !validators.ValidateSQN(sub.Sqn) || !validators.ValidateAMF(sub.Amf) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed"})
		return
	}

	if err := h.Client.CreateSubscriber(&sub); err != nil {
		slog.Error("Failed to create subscriber", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subscriber"})
		return
	}

	c.Header("HX-Trigger", "refreshList") // Trigger list refresh
	c.Status(http.StatusCreated)
}

func (h *SubscriberHandler) UpdateSubscriber(c *gin.Context) {
	imsi := c.Param("imsi")
	var sub client.Subscriber
	if err := c.ShouldBindJSON(&sub); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// IMSI in body is ignored for update, but we validate other fields
	if !validators.ValidateKey(sub.Ki) || !validators.ValidateOPC(sub.Opc) || !validators.ValidateSQN(sub.Sqn) || !validators.ValidateAMF(sub.Amf) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed"})
		return
	}

	if err := h.Client.UpdateSubscriber(imsi, &sub); err != nil {
		slog.Error("Failed to update subscriber", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subscriber"})
		return
	}

	c.Header("HX-Trigger", "refreshList")
	c.Status(http.StatusOK)
}

func (h *SubscriberHandler) DeleteSubscriber(c *gin.Context) {
	imsi := c.Param("imsi")
	if err := h.Client.DeleteSubscriber(imsi); err != nil {
		slog.Error("Failed to delete subscriber", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subscriber"})
		return
	}

	c.Header("HX-Trigger", "refreshList")
	c.Status(http.StatusNoContent)
}

func (h *SubscriberHandler) GetSubscriber(c *gin.Context) {
	imsi := c.Param("imsi")
	sub, err := h.Client.GetSubscriber(imsi)
	if err != nil {
		slog.Error("Failed to get subscriber", "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	if sub == nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, sub)
}
