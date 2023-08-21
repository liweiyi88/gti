package web

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/liweiyi88/gti/config"
	"github.com/liweiyi88/gti/database"
	"github.com/liweiyi88/gti/global"
	"github.com/liweiyi88/gti/web/controller"
	"github.com/liweiyi88/gti/web/middleware"
	"golang.org/x/exp/slog"
)

type Controllers struct {
	repositoryController *controller.RepositoryController
	tagController        *controller.TagController
	securityController   *controller.SecurityController
}

func initControllers(repositories *global.Repositories) *Controllers {
	return &Controllers{
		repositoryController: controller.NewRepositoryController(repositories.GhRepositoryRepo),
		tagController:        controller.NewTagController(repositories.TagRepo),
		securityController:   controller.NewSecurityController(repositories.UserRepo),
	}
}

func setupRouter(ctx context.Context) (*gin.Engine, *sql.DB) {
	db := database.GetInstance(ctx)
	repositories := global.InitRepositories(db)
	controllers := initControllers(repositories)

	gin.SetMode(config.GinMode)
	router := gin.Default()

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = config.CORSAllowOrigins
	corsConfig.AllowCredentials = true

	router.Use(cors.New(corsConfig))

	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	router.POST("/login", controllers.securityController.Login)

	router.GET("/api/tags", controllers.tagController.List)

	// Protected routes.
	auth := router.Group("/api")
	auth.Use(middleware.JwtAuth())
	auth.POST("/tags", controllers.tagController.Save)
	auth.POST("/repositories/:id/tags", controllers.repositoryController.SaveTags)

	return router, db
}

func Server() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	router, db := setupRouter(ctx)

	defer func() {
		err := db.Close()

		if err != nil {
			slog.Error("failed to close db: %v", err)
		}

		stop()
	}()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-ctx.Done()

	stop()

	slog.Info("shutting down gracefully...")

	// The context is used to inform the server it has 10 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("server forced to shutdown: ", err)
	}

	slog.Info("server exit")
}
