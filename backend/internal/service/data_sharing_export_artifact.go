package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/klauspost/compress/zstd"
)

const dataShareExportArtifactDownloadTicketTTL = 10 * time.Minute

// CreateExportArtifact 创建预生成导出文件任务，并在后台执行真实文件生成。
func (s *DataSharingService) CreateExportArtifact(ctx context.Context, input DataShareExportArtifactCreateInput) (*DataShareExportArtifact, error) {
	if s == nil || s.exportArtifactRepo == nil {
		return nil, ErrDataShareExportArtifactStorageInvalid
	}
	if err := validateDataShareExportArtifactInput(input); err != nil {
		return nil, err
	}
	encoding := normalizeDataShareExportEncoding(input.Encoding)
	filename := normalizeDataShareExportFilename(input.Filename, encoding)
	artifact, err := s.exportArtifactRepo.Create(ctx, &DataShareExportArtifact{
		Status:   DataShareExportArtifactStatusPending,
		Filename: filename,
		Encoding: string(encoding),
		Filters:  input.Filters,
	})
	if err != nil {
		return nil, err
	}
	// 生成任务不依赖发起请求生命周期，避免浏览器或代理断开后任务被取消。
	go s.generateExportArtifact(context.Background(), artifact.ID)
	return artifact, nil
}

// ListExportArtifacts 分页列出预生成导出文件任务。
func (s *DataSharingService) ListExportArtifacts(ctx context.Context, params pagination.PaginationParams) ([]DataShareExportArtifact, *pagination.PaginationResult, error) {
	if s == nil || s.exportArtifactRepo == nil {
		return nil, nil, ErrDataShareExportArtifactStorageInvalid
	}
	return s.exportArtifactRepo.List(ctx, params)
}

// GetExportArtifact 返回单个预生成导出文件任务。
func (s *DataSharingService) GetExportArtifact(ctx context.Context, id int64) (*DataShareExportArtifact, error) {
	if s == nil || s.exportArtifactRepo == nil {
		return nil, ErrDataShareExportArtifactStorageInvalid
	}
	if id <= 0 {
		return nil, ErrDataShareExportArtifactNotFound
	}
	return s.exportArtifactRepo.Get(ctx, id)
}

// DeleteExportArtifact 删除本地文件并将任务标记为 deleted。
func (s *DataSharingService) DeleteExportArtifact(ctx context.Context, id int64) error {
	artifact, err := s.GetExportArtifact(ctx, id)
	if err != nil {
		return err
	}
	if artifact.Status == DataShareExportArtifactStatusDeleted {
		return nil
	}
	if artifact.Status == DataShareExportArtifactStatusPending || artifact.Status == DataShareExportArtifactStatusRunning {
		return ErrDataShareExportArtifactNotReady
	}
	if artifact.StoragePath != "" {
		if err := s.validateExportArtifactPath(artifact.StoragePath); err != nil {
			return err
		}
		if err := os.Remove(artifact.StoragePath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return s.exportArtifactRepo.MarkDeleted(ctx, id)
}

// CreateExportArtifactDownloadTicket 为已完成的预生成文件签发短期下载票据。
func (s *DataSharingService) CreateExportArtifactDownloadTicket(ctx context.Context, id int64) (*DataShareExportTicket, error) {
	artifact, err := s.GetExportArtifact(ctx, id)
	if err != nil {
		return nil, err
	}
	if artifact.Status == DataShareExportArtifactStatusDeleted {
		return nil, ErrDataShareExportArtifactDeleted
	}
	if artifact.Status != DataShareExportArtifactStatusCompleted || strings.TrimSpace(artifact.StoragePath) == "" {
		return nil, ErrDataShareExportArtifactNotReady
	}
	key, err := s.exportTicketSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(dataShareExportArtifactDownloadTicketTTL)
	encoding := normalizeDataShareExportEncoding(DataShareExportEncoding(artifact.Encoding))
	claims := DataShareExportTicketClaims{
		Scope:      DataShareExportScopeAdmin,
		ArtifactID: artifact.ID,
		Filename:   normalizeDataShareExportFilename(artifact.Filename, encoding),
		Encoding:   encoding,
		ExpiresAt:  expiresAt.Unix(),
	}
	token, err := signDataShareExportTicket(claims, key)
	if err != nil {
		return nil, err
	}
	return &DataShareExportTicket{
		Token:       token,
		DownloadURL: "/api/v1/admin/data-sharing/exports/download?ticket=" + token,
		Filename:    claims.Filename,
		Encoding:    string(claims.Encoding),
		ExpiresAt:   expiresAt,
	}, nil
}

// OpenExportArtifactDownload 校验票据并打开已完成的本地导出文件。
func (s *DataSharingService) OpenExportArtifactDownload(ctx context.Context, token string) (io.ReadCloser, *DataShareExportArtifact, error) {
	key, err := s.exportTicketSigningKey(ctx)
	if err != nil {
		return nil, nil, err
	}
	claims, err := parseDataShareExportTicket(strings.TrimSpace(token), key)
	if err != nil {
		return nil, nil, err
	}
	if claims.Scope != DataShareExportScopeAdmin || claims.ArtifactID <= 0 {
		return nil, nil, ErrDataShareExportTicketForbidden
	}
	if claims.ExpiresAt <= 0 || time.Now().Unix() > claims.ExpiresAt {
		return nil, nil, ErrDataShareExportTicketInvalid
	}
	artifact, err := s.GetExportArtifact(ctx, claims.ArtifactID)
	if err != nil {
		return nil, nil, err
	}
	if artifact.Status == DataShareExportArtifactStatusDeleted {
		return nil, nil, ErrDataShareExportArtifactDeleted
	}
	if artifact.Status != DataShareExportArtifactStatusCompleted {
		return nil, nil, ErrDataShareExportArtifactNotReady
	}
	if err := s.validateExportArtifactPath(artifact.StoragePath); err != nil {
		return nil, nil, err
	}
	f, err := os.Open(artifact.StoragePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrDataShareExportArtifactNotFound
		}
		return nil, nil, err
	}
	return f, artifact, nil
}

func validateDataShareExportArtifactInput(input DataShareExportArtifactCreateInput) error {
	if input.Filters.SelectAll {
		return nil
	}
	if len(input.Filters.IDs) == 0 {
		return ErrDataShareExportTicketInvalid
	}
	return nil
}

func (s *DataSharingService) generateExportArtifact(ctx context.Context, id int64) {
	if err := s.generateExportArtifactWithError(ctx, id); err != nil {
		slog.Warn("data sharing: generate export artifact failed", "artifact_id", id, "error", err)
		if s != nil && s.exportArtifactRepo != nil {
			_ = s.exportArtifactRepo.MarkFailed(context.Background(), id, err.Error())
		}
	}
}

func (s *DataSharingService) generateExportArtifactWithError(ctx context.Context, id int64) error {
	artifact, err := s.GetExportArtifact(ctx, id)
	if err != nil {
		return err
	}
	if err := s.exportArtifactRepo.MarkRunning(ctx, id); err != nil {
		return err
	}
	dir, err := s.ensureExportStorageDir()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, fmt.Sprintf(".artifact-%d-*.tmp", id))
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		_ = tmp.Close()
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	fileWriter := &dataShareExportArtifactFileWriter{w: tmp, hash: sha256.New()}
	encoding := normalizeDataShareExportEncoding(DataShareExportEncoding(artifact.Encoding))
	switch encoding {
	case DataShareExportEncodingJSON, DataShareExportEncodingJSONL:
		lineCounter := &dataShareExportArtifactLineCountingWriter{w: fileWriter}
		if err := s.ExportJSONL(ctx, lineCounter, artifact.Filters, false); err != nil {
			return err
		}
		fileWriter.lines = lineCounter.lines
	default:
		zw, err := zstd.NewWriter(fileWriter)
		if err != nil {
			return err
		}
		lineCounter := &dataShareExportArtifactLineCountingWriter{w: zw}
		if err := s.ExportJSONL(ctx, lineCounter, artifact.Filters, false); err != nil {
			_ = zw.Close()
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		fileWriter.lines = lineCounter.lines
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	finalPath := filepath.Join(dir, dataShareExportArtifactStorageFilename(id, artifact.Filename))
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}
	cleanupTmp = false
	return s.exportArtifactRepo.MarkCompleted(ctx, id, finalPath, fileWriter.lines, fileWriter.bytes, fileWriter.SumHex())
}

func (s *DataSharingService) ensureExportStorageDir() (string, error) {
	dir := strings.TrimSpace(s.exportStorageDir)
	if dir == "" {
		dir = filepath.Join(".", "data", "data-sharing-exports")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", err
	}
	return abs, nil
}

func (s *DataSharingService) validateExportArtifactPath(path string) error {
	base, err := s.ensureExportStorageDir()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return ErrDataShareExportArtifactStorageInvalid
	}
	return nil
}

func dataShareExportArtifactStorageFilename(id int64, filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "data-sharing-export"
	}
	filename = strings.NewReplacer("/", "-", "\\", "-", "\x00", "").Replace(filename)
	return fmt.Sprintf("%d-%s", id, filename)
}

type dataShareExportArtifactFileWriter struct {
	w     io.Writer
	hash  hash.Hash
	bytes int64
	lines int64
}

func (w *dataShareExportArtifactFileWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		chunk := p[:n]
		w.bytes += int64(n)
		_, _ = w.hash.Write(chunk)
	}
	return n, err
}

func (w *dataShareExportArtifactFileWriter) SumHex() string {
	if w == nil || w.hash == nil {
		return ""
	}
	return hex.EncodeToString(w.hash.Sum(nil))
}

type dataShareExportArtifactLineCountingWriter struct {
	w     io.Writer
	lines int64
}

func (w *dataShareExportArtifactLineCountingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		for _, b := range p[:n] {
			if b == '\n' {
				w.lines++
			}
		}
	}
	return n, err
}
