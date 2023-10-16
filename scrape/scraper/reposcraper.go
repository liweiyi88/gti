package scraper

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/liweiyi88/trendshift-backend/model"
	"github.com/liweiyi88/trendshift-backend/utils/dbutils"
)

const ghTrendScrapePath = ".Box-row .h3.lh-condensed a[href]"
const ghTrendScrapeBaseURL = "https://github.com/trending"

type TrendingRepositoryScraper struct {
	url, path string
	trendRepo *model.TrendingRepositoryRepo
}

func NewTrendingRepositoryScraper(trendRepo *model.TrendingRepositoryRepo) *TrendingRepositoryScraper {
	return &TrendingRepositoryScraper{
		url:       ghTrendScrapeBaseURL,
		path:      ghTrendScrapePath,
		trendRepo: trendRepo,
	}
}

func (gh *TrendingRepositoryScraper) GetType() string {
	return "repository"
}

func (gh *TrendingRepositoryScraper) Scrape(ctx context.Context, language string) error {
	c := colly.NewCollector()

	repos := make([]string, 0)

	c.OnHTML(gh.path, func(e *colly.HTMLElement) {
		link := e.Attr("href")

		if strings.HasPrefix(link, "/") {
			link = strings.TrimLeft(link, "/")
		}

		repos = append(repos, link)
	})

	c.OnRequest(func(r *colly.Request) {
		// fmt.Printf("scraping: %s \n", r.URL.String())
	})

	c.Visit(gh.getTrendPageUrl(language))

	now := time.Now()
	rankedTrendingRepo, err := gh.trendRepo.FindRankedTrendingRepoByDate(ctx, now, language)

	if err != nil {
		return fmt.Errorf("failed to retrieve ranked trending repositoris: %v", err)
	}

	for index, repo := range repos {
		rank := index + 1

		trendingRepo, ok := rankedTrendingRepo[rank]

		if ok && trendingRepo.RepoFullName != "" {
			// if trending repo exist, do update.
			trendingRepo.RepoFullName = repo
			trendingRepo.ScrapedAt, trendingRepo.TrendDate = now, now

			gh.trendRepo.Update(ctx, trendingRepo)
		} else {
			// trending repo does not exist, do insert.
			trendingRepo := model.TrendingRepository{
				RepoFullName: repo,
				ScrapedAt:    now,
				TrendDate:    now,
				Rank:         rank,
			}

			if language != "" {
				trendingRepo.Language = dbutils.NullString{
					NullString: sql.NullString{String: strings.ToLower(language),
						Valid: true,
					},
				}
			} else {
				trendingRepo.Language = dbutils.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					}}
			}

			err = gh.trendRepo.Save(ctx, trendingRepo)
		}
	}

	return err
}

func (gh *TrendingRepositoryScraper) getTrendPageUrl(language string) string {
	language = strings.TrimSpace(language)

	if language != "" {
		return fmt.Sprintf("%s/%s?since=daily", gh.url, url.QueryEscape(language))
	}

	return gh.url
}
