package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"log/slog"

	"github.com/getsentry/sentry-go"
	"github.com/liweiyi88/trendshift-backend/config"
	"github.com/liweiyi88/trendshift-backend/database"
	"github.com/liweiyi88/trendshift-backend/github"
	"github.com/liweiyi88/trendshift-backend/model"
	"github.com/liweiyi88/trendshift-backend/model/opt"
	"github.com/liweiyi88/trendshift-backend/utils/dbutils"
	"github.com/spf13/cobra"
)

var start string
var end string
var limit int

// If run as cronjob, a suggested command to avoid sending too many requests to GitHub is
// `sync [repository|developer] --end=-2d --limit=500` and run it hourly.
func init() {
	rootCmd.AddCommand(gihtubSyncCmd)

	gihtubSyncCmd.Flags().StringVarP(&start, "start", "s", "", "--start \"2023-01-06 14:35:00\" ")
	gihtubSyncCmd.Flags().StringVarP(&end, "end", "e", "", "--end \"2023-10-06 14:35:00\", --end=-2d or --end=2h, `d` for days, `h` for hours ")
	gihtubSyncCmd.Flags().IntVarP(&limit, "limit", "l", 0, "--limit=100")
}

var gihtubSyncCmd = &cobra.Command{
	Use:   "sync [repository|developer]",
	Short: "Sync the latest repositories or developers details from GitHub",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		config.Init()

		action := args[0]
		ctx, stop := context.WithCancel(context.Background())
		db := database.GetInstance(ctx)
		gh := github.NewClient(config.GitHubToken)

		defer func() {
			err := db.Close()

			if err != nil {
				slog.Error("failed to close db", slog.Any("error", err))
				sentry.CaptureException(err)
			}

			stop()
			sentry.Flush(2 * time.Second)
		}()

		if start != "" {
			_, err := time.Parse("2006-01-02 15:04:05", start)
			if err != nil {
				slog.Error("failed to parse start time", slog.Any("error", err))
				sentry.CaptureException(err)
				return
			}
		}

		endDateTime, err := parseEndDateTimeOption(end)
		if err != nil {
			slog.Error("failed to parse end date time", slog.Any("error", err))
			sentry.CaptureException(err)
			return
		}

		appSignal := make(chan os.Signal, 3)
		signal.Notify(appSignal, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-appSignal
			stop()
		}()

		qb := dbutils.NewQueryBuilder()
		repositoryRepo := model.NewGhRepositoryRepo(db, qb)
		developerRepo := model.NewDeveloperRepo(db, qb)
		handler := github.NewSyncHandler(db, repositoryRepo, developerRepo, gh)
		err = handler.Handle(ctx, action, opt.Start(start), opt.End(endDateTime), opt.Limit(limit))

		if err != nil {
			slog.Error("failed to handle sync action", slog.Any("error", err))
			sentry.CaptureException(err)
		}
	},
}

func parseEndDateTimeOption(end string) (string, error) {
	end = strings.TrimSpace(end)

	if end == "" {
		return "", nil
	}

	var unit string
	var numberPart string

	if strings.HasSuffix(end, "d") {
		unit = "d"
		numberPart, _ = strings.CutSuffix(end, "d")
	} else if strings.HasSuffix(end, "h") {
		unit = "h"
		numberPart, _ = strings.CutSuffix(end, "h")
	}

	number, err := strconv.Atoi(numberPart)
	if err == nil && unit != "" {
		now := time.Now()

		if unit == "d" {
			endTime := now.Add(time.Duration(number) * 24 * time.Hour)
			end = endTime.Format("2006-01-02 15:04:05")
			return end, nil
		} else if unit == "h" {
			endTime := now.Add(time.Duration(number) * time.Hour)

			fmt.Println("end time", endTime.Format("2006-01-02 15:04:05"))
			end = endTime.Format("2006-01-02 15:04:05")
			return end, nil
		}
	}

	_, err = time.Parse("2006-01-02 15:04:05", end)
	return end, err
}
