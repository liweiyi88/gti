package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"log/slog"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/liweiyi88/trendshift-backend/config"
	"github.com/liweiyi88/trendshift-backend/database"
	"github.com/liweiyi88/trendshift-backend/github"
	"github.com/liweiyi88/trendshift-backend/global"
	"github.com/liweiyi88/trendshift-backend/scrape"
	"github.com/liweiyi88/trendshift-backend/search"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(scrapeCmd)
}

var scrapeCmd = &cobra.Command{
	Use:   "scrape [repository|developer]",
	Short: "Scrape trending repositories or trending developers form GitHub trending page.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := args[0]
		config.Init()

		search := search.NewSearch()
		ctx, stop := context.WithCancel(context.Background())
		db := database.GetInstance(ctx)
		repositories := global.InitRepositories(db)
		gh := github.NewClient(config.GitHubToken)

		handler := scrape.NewScrapeHandler(repositories, search, gh)

		defer func() {
			err := db.Close()

			if err != nil {
				slog.Error("failed to close db", slog.Any("error", err))
			}

			stop()
		}()

		appSignal := make(chan os.Signal, 3)
		signal.Notify(appSignal, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-appSignal
			stop()
		}()

		return handler.Handle(ctx, action)
	},
}
