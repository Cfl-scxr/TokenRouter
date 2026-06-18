package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/service"
)

type dataShareExportArtifactRepository struct {
	db *sql.DB
}

// NewDataShareExportArtifactRepository 创建数据共享预生成导出文件仓储。
func NewDataShareExportArtifactRepository(db *sql.DB) service.DataShareExportArtifactRepository {
	return &dataShareExportArtifactRepository{db: db}
}

func (r *dataShareExportArtifactRepository) Create(ctx context.Context, artifact *service.DataShareExportArtifact) (*service.DataShareExportArtifact, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrDataShareExportArtifactStorageInvalid
	}
	if artifact == nil {
		return nil, service.ErrDataShareExportArtifactNotFound
	}
	filters, err := json.Marshal(artifact.Filters)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO data_share_export_artifacts (status, filename, encoding, filters)
		VALUES ($1, $2, $3, $4)
		RETURNING id, status, filename, storage_path, encoding, filters, session_count, file_size, sha256,
			error_message, created_at, started_at, completed_at, deleted_at, updated_at
	`, artifact.Status, artifact.Filename, artifact.Encoding, filters)
	return scanDataShareExportArtifact(row)
}

func (r *dataShareExportArtifactRepository) Get(ctx context.Context, id int64) (*service.DataShareExportArtifact, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrDataShareExportArtifactStorageInvalid
	}
	if id <= 0 {
		return nil, service.ErrDataShareExportArtifactNotFound
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, status, filename, storage_path, encoding, filters, session_count, file_size, sha256,
			error_message, created_at, started_at, completed_at, deleted_at, updated_at
		FROM data_share_export_artifacts
		WHERE id = $1
	`, id)
	return scanDataShareExportArtifact(row)
}

func (r *dataShareExportArtifactRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.DataShareExportArtifact, *pagination.PaginationResult, error) {
	if r == nil || r.db == nil {
		return nil, nil, service.ErrDataShareExportArtifactStorageInvalid
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit()
	sortBy := dataShareExportArtifactSortField(params.SortBy)
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_share_export_artifacts`).Scan(&total); err != nil {
		return nil, nil, err
	}
	query := fmt.Sprintf(`
		SELECT id, status, filename, storage_path, encoding, filters, session_count, file_size, sha256,
			error_message, created_at, started_at, completed_at, deleted_at, updated_at
		FROM data_share_export_artifacts
		ORDER BY %s %s, id DESC
		LIMIT $1 OFFSET $2
	`, sortBy, sortOrder)
	rows, err := r.db.QueryContext(ctx, query, limit, (page-1)*limit)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.DataShareExportArtifact, 0, limit)
	for rows.Next() {
		item, err := scanDataShareExportArtifact(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	pages := int((total + int64(limit) - 1) / int64(limit))
	if pages < 1 {
		pages = 1
	}
	return items, &pagination.PaginationResult{Total: total, Page: page, PageSize: limit, Pages: pages}, nil
}

func (r *dataShareExportArtifactRepository) MarkRunning(ctx context.Context, id int64) error {
	return r.execArtifactUpdate(ctx, `
		UPDATE data_share_export_artifacts
		SET status = 'running', started_at = COALESCE(started_at, NOW()), updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'running')
	`, id)
}

func (r *dataShareExportArtifactRepository) MarkCompleted(ctx context.Context, id int64, storagePath string, sessionCount int64, fileSize int64, sha256 string) error {
	return r.execArtifactUpdate(ctx, `
		UPDATE data_share_export_artifacts
		SET status = 'completed', storage_path = $2, session_count = $3, file_size = $4, sha256 = $5,
			error_message = '', completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'running')
	`, id, storagePath, sessionCount, fileSize, sha256)
}

func (r *dataShareExportArtifactRepository) MarkFailed(ctx context.Context, id int64, errorMessage string) error {
	errorMessage = strings.TrimSpace(errorMessage)
	if len(errorMessage) > 4000 {
		errorMessage = errorMessage[:4000]
	}
	return r.execArtifactUpdate(ctx, `
		UPDATE data_share_export_artifacts
		SET status = 'failed', error_message = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status <> 'deleted'
	`, id, errorMessage)
}

func (r *dataShareExportArtifactRepository) MarkInterruptedFailed(ctx context.Context, errorMessage string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, service.ErrDataShareExportArtifactStorageInvalid
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if len(errorMessage) > 4000 {
		errorMessage = errorMessage[:4000]
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE data_share_export_artifacts
		SET status = 'failed', error_message = $1, completed_at = NOW(), updated_at = NOW()
		WHERE status IN ('pending', 'running')
	`, errorMessage)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *dataShareExportArtifactRepository) MarkDeleted(ctx context.Context, id int64) error {
	return r.execArtifactUpdate(ctx, `
		UPDATE data_share_export_artifacts
		SET status = 'deleted', storage_path = '', deleted_at = COALESCE(deleted_at, NOW()), updated_at = NOW()
		WHERE id = $1
	`, id)
}

func (r *dataShareExportArtifactRepository) execArtifactUpdate(ctx context.Context, query string, args ...any) error {
	if r == nil || r.db == nil {
		return service.ErrDataShareExportArtifactStorageInvalid
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrDataShareExportArtifactNotFound
	}
	return nil
}

type dataShareExportArtifactScanner interface {
	Scan(dest ...any) error
}

func scanDataShareExportArtifact(scanner dataShareExportArtifactScanner) (*service.DataShareExportArtifact, error) {
	var (
		item        service.DataShareExportArtifact
		status      string
		filtersJSON []byte
		startedAt   sql.NullTime
		completedAt sql.NullTime
		deletedAt   sql.NullTime
	)
	err := scanner.Scan(
		&item.ID,
		&status,
		&item.Filename,
		&item.StoragePath,
		&item.Encoding,
		&filtersJSON,
		&item.SessionCount,
		&item.FileSize,
		&item.SHA256,
		&item.ErrorMessage,
		&item.CreatedAt,
		&startedAt,
		&completedAt,
		&deletedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrDataShareExportArtifactNotFound
		}
		return nil, err
	}
	item.Status = service.DataShareExportArtifactStatus(status)
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &item.Filters); err != nil {
			return nil, err
		}
	}
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	if deletedAt.Valid {
		item.DeletedAt = &deletedAt.Time
	}
	return &item, nil
}

func dataShareExportArtifactSortField(raw string) string {
	switch strings.TrimSpace(raw) {
	case "updated_at":
		return "updated_at"
	case "completed_at":
		return "completed_at"
	case "file_size":
		return "file_size"
	case "session_count":
		return "session_count"
	default:
		return "created_at"
	}
}
