package handlers

import (
	"net/http"

	"aka-webgui/internal/config"
	"aka-webgui/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	Config *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{Config: cfg}
}

func (h *AuthHandler) ShowLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"Title": "Login - AKA-Only-Server Web GUI",
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == h.Config.AuthUsername && password == h.Config.AuthPassword {
		middleware.SetSession(c, h.Config)
		c.Redirect(http.StatusFound, "/")
	} else {
		c.HTML(http.StatusOK, "login.html", gin.H{
			"Title": "Login - AKA-Only-Server Web GUI",
			"Error": "Invalid username or password",
		})
	}
}

func (h *AuthHandler) Logout(c *gin.Context) {
	middleware.ClearSession(c)
	c.Redirect(http.StatusFound, "/login")
}
