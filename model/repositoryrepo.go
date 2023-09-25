package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/liweiyi88/gti/database"
	"github.com/liweiyi88/gti/dbutils"
)

type TrendingRepositoryResponse struct {
	GhRepository
	BestRanking   int `json:"best_ranking"`   // non db column field
	FeaturedCount int `json:"featured_count"` // non db column field
}

type GhRepositoryRepo struct {
	db database.DB
	qb *dbutils.QueryBuilder
}

func NewGhRepositoryRepo(db database.DB, qb *dbutils.QueryBuilder) *GhRepositoryRepo {
	return &GhRepositoryRepo{
		db: db,
		qb: qb,
	}
}

func (gr *GhRepositoryRepo) FindById(ctx context.Context, id int) (GhRepository, error) {
	query := "SELECT * FROM repositories WHERE id = ?"

	var ghr GhRepository

	row := gr.db.QueryRowContext(ctx, query, id)

	if err := row.Scan(&ghr.Id, &ghr.GhrId, &ghr.Stars, &ghr.Forks, &ghr.FullName, &ghr.Language, &ghr.Owner.Name, &ghr.Owner.AvatarUrl, &ghr.CreatedAt, &ghr.UpdatedAt, &ghr.Description, &ghr.DefaultBranch); err != nil {
		return ghr, err
	}

	return ghr, nil
}

func (gr *GhRepositoryRepo) FindByName(ctx context.Context, name string) (GhRepository, error) {
	query := "SELECT * FROM repositories WHERE full_name = ?"

	var ghr GhRepository

	row := gr.db.QueryRowContext(ctx, query, name)

	if err := row.Scan(&ghr.Id, &ghr.GhrId, &ghr.Stars, &ghr.Forks, &ghr.FullName, &ghr.Language, &ghr.Owner.Name, &ghr.Owner.AvatarUrl, &ghr.CreatedAt, &ghr.UpdatedAt, &ghr.Description, &ghr.DefaultBranch); err != nil {
		return ghr, err
	}

	return ghr, nil
}

func (gr *GhRepositoryRepo) FindAll(ctx context.Context, start string, end string, limit int) ([]GhRepository, error) {
	var args []any
	query := "select * from repositories"

	var criteria []string
	if start != "" {
		criteria = append(criteria, "updated_at > ?")
		args = append(args, start)
	}

	if end != "" {
		criteria = append(criteria, "updated_at <= ?")
		args = append(args, end)
	}

	if len(criteria) > 0 {
		query = query + " where " + strings.Join(criteria, " and ")
	}

	if limit > 0 {
		query = query + " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := gr.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var repositories []GhRepository

	for rows.Next() {
		var ghr GhRepository

		if err := rows.Scan(&ghr.Id, &ghr.GhrId, &ghr.Stars, &ghr.Forks, &ghr.FullName, &ghr.Language, &ghr.Owner.Name, &ghr.Owner.AvatarUrl, &ghr.CreatedAt, &ghr.UpdatedAt, &ghr.Description, &ghr.DefaultBranch); err != nil {
			return nil, err
		}

		repositories = append(repositories, ghr)
	}

	if err = rows.Err(); err != nil {
		return repositories, err
	}

	return repositories, nil
}

func (gr *GhRepositoryRepo) FindAllWithTags(ctx context.Context, filter string) ([]*GhRepository, error) {
	var query string
	var args []any

	if filter == "today" {
		query = "select repositories.*, tags.id as tag_id, tags.`name` as tag_name  from repositories left join repositories_tags ON repositories.id = repositories_tags.repository_id left join tags on repositories_tags.tag_id = tags.id join trending_repositories on repositories.id = trending_repositories.repository_id where trend_date = ? group by repositories.full_name, tags.id"
		args = append(args, time.Now().Format("2006-01-02"))
	} else {
		query = "select repositories.*, tags.id as tag_id, tags.`name` as tag_name from repositories left join repositories_tags ON repositories.id = repositories_tags.repository_id left join tags on repositories_tags.tag_id = tags.id"
	}

	rows, err := gr.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	collectionMap := dbutils.NewCollectionMap[int, *GhRepository]()

	for rows.Next() {
		var ghr GhRepository
		var tagId dbutils.NullInt64
		var tagName dbutils.NullString

		if err := rows.Scan(&ghr.Id, &ghr.GhrId, &ghr.Stars, &ghr.Forks, &ghr.FullName, &ghr.Language, &ghr.Owner.Name, &ghr.Owner.AvatarUrl, &ghr.CreatedAt, &ghr.UpdatedAt, &ghr.Description, &ghr.DefaultBranch, &tagId, &tagName); err != nil {
			return nil, err
		}

		if !collectionMap.Has(ghr.Id) {
			ghr.Tags = make([]Tag, 0)
			collectionMap.Set(ghr.Id, &ghr)
		}

		if tagId.Valid && tagName.Valid {
			tag := Tag{
				Id:   int(tagId.Int64),
				Name: tagName.String,
			}

			repo := collectionMap.Get(ghr.Id)
			repo.Tags = append(repo.Tags, tag)
			collectionMap.Set(ghr.Id, repo)
		}
	}

	return collectionMap.All(), nil
}

func (gr *GhRepositoryRepo) FindTrendingRepositories(ctx context.Context, language string, limit int, dataRange int) ([]TrendingRepositoryResponse, error) {
	lang := strings.TrimSpace(language)

	query := "select repositories.*, count(*) as count, min(trending_repositories.`rank`) as best_ranking from repositories join trending_repositories on repositories.id = trending_repositories.repository_id"

	qb := gr.qb
	qb.Query(query)

	qb.OrderBy("count", "DESC")
	qb.OrderBy("best_ranking", "ASC")
	qb.OrderBy("repositories.id", "ASC")

	if lang != "" {
		qb.Where("`trending_repositories`.`language` = ?", lang)
	} else {
		qb.Where("`trending_repositories`.`language` is null", nil)
	}

	if dataRange > 0 {
		since := time.Now().AddDate(0, 0, -dataRange)
		qb.Where("`trending_repositories`.`trend_date` > ?", since.Format("2006-01-02"))
	}

	if limit > 0 {
		qb.Limit(limit)
	}

	qb.GroupBy("repositories.id")

	q, args := qb.GetQuery()

	rows, err := gr.db.QueryContext(ctx, q, args...)

	if err != nil {
		return nil, fmt.Errorf("failed to query trending repositories: %v", err)
	}

	defer rows.Close()

	var repositories []TrendingRepositoryResponse

	for rows.Next() {
		var trr TrendingRepositoryResponse

		if err := rows.Scan(
			&trr.Id,
			&trr.GhrId,
			&trr.Stars,
			&trr.Forks,
			&trr.FullName,
			&trr.Language,
			&trr.Owner.Name,
			&trr.Owner.AvatarUrl,
			&trr.CreatedAt,
			&trr.UpdatedAt,
			&trr.Description,
			&trr.DefaultBranch,
			&trr.FeaturedCount,
			&trr.BestRanking); err != nil {
			return nil, err

		}
		repositories = append(repositories, trr)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return repositories, nil
}

func (gr *GhRepositoryRepo) FindRepositoriesByNames(ctx context.Context, names []string) ([]GhRepository, error) {
	ghRepos := make([]GhRepository, 0)

	if len(names) == 0 {
		return ghRepos, nil
	}

	query := fmt.Sprintf("SELECT * FROM repositories WHERE full_name IN ('%s')", strings.Join(names, "','"))

	rows, err := gr.db.QueryContext(ctx, query)

	if err != nil {
		return nil, fmt.Errorf("failed to run the select query: %s, error: %v", query, err)
	}

	defer rows.Close()

	for rows.Next() {
		var ghr GhRepository

		if err := rows.Scan(
			&ghr.Id,
			&ghr.GhrId,
			&ghr.Stars,
			&ghr.Forks,
			&ghr.FullName,
			&ghr.Language,
			&ghr.Owner.Name,
			&ghr.Owner.AvatarUrl,
			&ghr.CreatedAt,
			&ghr.UpdatedAt,
			&ghr.Description,
			&ghr.DefaultBranch); err != nil {
			return ghRepos, err
		}

		ghRepos = append(ghRepos, ghr)
	}

	if err = rows.Err(); err != nil {
		return ghRepos, err
	}

	return ghRepos, nil
}

func (gr *GhRepositoryRepo) Save(ctx context.Context, ghRepo GhRepository) (int64, error) {
	query := "INSERT INTO `repositories` (`full_name`, `ghr_id`, stars, forks, `language`, `owner`, `owner_avatar_url`, `description`, `default_branch`, `created_at`, `updated_at`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	var lastInsertId int64

	createdAt, updatedAt := time.Now(), time.Now()

	result, err := gr.db.ExecContext(ctx, query,
		ghRepo.FullName,
		ghRepo.GhrId,
		ghRepo.Stars,
		ghRepo.Forks,
		ghRepo.Language,
		ghRepo.Owner.Name,
		ghRepo.Owner.AvatarUrl,
		ghRepo.GetDescription(),
		ghRepo.DefaultBranch,
		createdAt.Format("2006-01-02 15:04:05"),
		updatedAt.Format("2006-01-02 15:04:05"),
	)

	if err != nil {
		return lastInsertId, fmt.Errorf("failed to exec insert repositories query to db, error: %v", err)
	}

	lastInsertId, err = result.LastInsertId()
	if err != nil {
		return lastInsertId, fmt.Errorf("failed to get repositories last insert id after insert, error: %v", err)
	}

	_, err = result.RowsAffected()

	if err != nil {
		return lastInsertId, fmt.Errorf("repositories insert rows affected returns error: %v", err)
	}

	return lastInsertId, nil
}

func (gr *GhRepositoryRepo) Update(ctx context.Context, ghRepo GhRepository) error {
	query := "UPDATE `repositories` SET full_name = ?, ghr_id = ?, stars = ?, forks = ?, language = ?, owner = ?, owner_avatar_url = ?, description = ?, default_branch = ?, updated_at = ? WHERE id = ?"

	updatedAt := time.Now()

	result, err := gr.db.ExecContext(ctx, query, ghRepo.FullName, ghRepo.GhrId, ghRepo.Stars, ghRepo.Forks, ghRepo.Language, ghRepo.Owner.Name, ghRepo.Owner.AvatarUrl, ghRepo.GetDescription(), ghRepo.DefaultBranch, updatedAt.Format("2006-01-02 15:04:05"), ghRepo.Id)

	if err != nil {
		return fmt.Errorf("failed to run repositories update query, gh repo id: %d, error: %v", ghRepo.Id, err)
	}

	n, err := result.RowsAffected()

	if err != nil {
		return fmt.Errorf("repositories update rows affected returns error: %v", err)
	}

	if n != 1 {
		return fmt.Errorf("unexpected number of rows affected after update: %d", n)
	}

	return nil
}

func (gr *GhRepositoryRepo) SaveTags(ctx context.Context, ghRepo GhRepository, tags []Tag) error {
	tx, err := gr.db.BeginTx(ctx, nil)

	if err != nil {
		return fmt.Errorf("failed to begin repository update tags transaction: %v", err)
	}

	defer tx.Rollback()
	query := "DELETE FROM `repositories_tags` WHERE repository_id = ?"

	_, err = tx.ExecContext(ctx, query, ghRepo.Id)

	if err != nil {
		return fmt.Errorf("failed to run delete repositories_tags query, repository id: %d, error: %v", ghRepo.Id, err)
	}

	for _, tag := range tags {
		query := "INSERT INTO `repositories_tags` (`repository_id`, `tag_id`) VALUES (?, ?)"

		result, err := tx.ExecContext(ctx, query, ghRepo.Id, tag.Id)

		if err != nil {
			return fmt.Errorf("failed to run insert repositories_tags query, repository id: %d, tag id: %d error: %v", ghRepo.Id, tag.Id, err)
		}

		_, err = result.RowsAffected()

		if err != nil {
			return fmt.Errorf("repositories_tags insert rows affected returns error: %v", err)
		}
	}

	return tx.Commit()
}
