package server

import (
	"log/slog"

	"aka-webgui/internal/client"
	"aka-webgui/internal/config"
	"aka-webgui/internal/server/handlers"
	"aka-webgui/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

func Run(cfg *config.Config) {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	// Custom logger middleware could be added here to use slog

	// Load templates
	r.LoadHTMLGlob("assets/templates/*.html")

	// Static files
	r.Static("/assets", "./assets")

	// Dependencies
	apiClient := client.New(cfg)
	authHandler := handlers.NewAuthHandler(cfg)
	subHandler := handlers.NewSubscriberHandler(apiClient)

	// Routes
	r.GET("/login", authHandler.ShowLogin)
	r.POST("/login", authHandler.Login)
	r.POST("/logout", authHandler.Logout)

	// Protected Routes
	authorized := r.Group("/")
	authorized.Use(middleware.AuthRequired(cfg))
	{
		authorized.GET("/", subHandler.Dashboard)
		authorized.GET("/subscribers/list", subHandler.ListSubscribers)
		authorized.POST("/subscribers", subHandler.CreateSubscriber)
		authorized.GET("/subscribers/:imsi", subHandler.GetSubscriber) // For editing
		authorized.PUT("/subscribers/:imsi", subHandler.UpdateSubscriber)
		authorized.DELETE("/subscribers/:imsi", subHandler.DeleteSubscriber)
	}

	slog.Info("Starting Web GUI", "addr", cfg.ListenAddr)
	if err := r.Run(cfg.ListenAddr); err != nil {
		slog.Error("Server failed to start", "error", err)
	}
}
