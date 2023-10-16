package controller

import (
	"net/http"

	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/liweiyi88/trendshift-backend/config"
	"github.com/liweiyi88/trendshift-backend/jwttoken"
	"github.com/liweiyi88/trendshift-backend/model"
)

type SecurityController struct {
	ur *model.UserRepo
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type tokenResponse struct {
	Token     string `json:"access_token"`
	ExpiredAt int64  `json:"expired_at"`
}

func NewSecurityController(ur *model.UserRepo) *SecurityController {
	return &SecurityController{
		ur: ur,
	}
}

func (sc *SecurityController) Login(c *gin.Context) {
	var request LoginRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		slog.Error(err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := sc.ur.FindByName(c, request.Username)

	if err != nil {
		slog.Error(err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad Request"})
		return
	}

	if !user.IsPasswordValid(request.Password) {
		slog.Error("invalid password")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad Request"})
		return
	}

	jwtsvc := jwttoken.NewTokenService(config.SignIngKey)
	tokenString, expiredAt, err := jwtsvc.Generate(user)

	if err != nil {
		slog.Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Error"})
		return
	}

	var response tokenResponse
	response.Token = tokenString
	response.ExpiredAt = expiredAt
	c.JSON(http.StatusOK, response)
}
