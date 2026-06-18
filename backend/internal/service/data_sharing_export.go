package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"
)

const (
	// dataShareDirectExportBatchSize 控制即时下载每次读取量，保持较低内存峰值。
	dataShareDirectExportBatchSize = 100
	// defaultDataShareExportBatchSize 是预生成导出文件的默认批次大小。
	defaultDataShareExportBatchSize = 500
	minDataShareExportBatchSize     = 50
	maxDataShareExportBatchSize     = 2000
)

// CreateExportTicket 为大文件下载签发短期票据，避免浏览器用 Blob 缓存完整导出文件。
func (s *DataSharingService) CreateExportTicket(ctx context.Context, req DataShareExportTicketRequest) (*DataShareExportTicket, error) {
	if err := validateDataShareExportTicketRequest(req); err != nil {
		return nil, err
	}
	key, err := s.exportTicketSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(dataShareExportTicketTTL)
	encoding := normalizeDataShareExportEncoding(req.Encoding)
	claims := DataShareExportTicketClaims{
		Scope:     req.Scope,
		UserID:    req.UserID,
		Filters:   req.Filters,
		Filename:  normalizeDataShareExportFilename(req.Filename, encoding),
		Encoding:  encoding,
		ExpiresAt: expiresAt.Unix(),
	}
	token, err := signDataShareExportTicket(claims, key)
	if err != nil {
		return nil, err
	}
	return &DataShareExportTicket{
		Token:       token,
		DownloadURL: dataShareExportDownloadURL(req.Scope, token),
		Filename:    claims.Filename,
		Encoding:    string(claims.Encoding),
		ExpiresAt:   expiresAt,
	}, nil
}

// ParseExportTicket 校验短期下载票据并返回导出上下文。
func (s *DataSharingService) ParseExportTicket(ctx context.Context, scope DataShareExportScope, token string) (*DataShareExportTicketClaims, error) {
	key, err := s.exportTicketSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := parseDataShareExportTicket(token, key)
	if err != nil {
		return nil, err
	}
	if claims.Scope != scope {
		return nil, ErrDataShareExportTicketForbidden
	}
	if claims.ExpiresAt <= 0 || time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrDataShareExportTicketInvalid
	}
	if err := validateDataShareExportTicketRequest(DataShareExportTicketRequest{
		Scope:    claims.Scope,
		UserID:   claims.UserID,
		Filters:  claims.Filters,
		Filename: claims.Filename,
		Encoding: claims.Encoding,
	}); err != nil {
		return nil, err
	}
	claims.Encoding = normalizeDataShareExportEncoding(claims.Encoding)
	claims.Filename = normalizeDataShareExportFilename(claims.Filename, claims.Encoding)
	return claims, nil
}

// ExportJSONL 导出选中的数据共享 session；显式选中的记录保留原始快照，不再因质量状态跳过。
func (s *DataSharingService) ExportJSONL(ctx context.Context, w io.Writer, filters DataShareSessionFilters, includeNonExportable bool) error {
	if s == nil || s.repo == nil {
		return ErrDataShareExportArtifactStorageInvalid
	}
	total, err := s.repo.Count(ctx, filters)
	if err != nil {
		return err
	}
	return s.exportJSONL(ctx, w, filters, includeNonExportable, total, dataShareDirectExportBatchSize, nil, nil)
}

func (s *DataSharingService) exportJSONL(ctx context.Context, w io.Writer, filters DataShareSessionFilters, includeNonExportable bool, total int64, batchSize int, progress func(processed int64, total int64), recorder DataShareExportDurationRecorder) error {
	_ = includeNonExportable
	if s == nil || s.repo == nil {
		return ErrDataShareExportArtifactStorageInvalid
	}
	batchSize = normalizeDataShareExportBatchSize(batchSize)
	var processed int64
	var cursor *DataShareSessionExportCursor
	for {
		items, nextCursor, err := s.repo.ListExportPayloadPage(ctx, filters, cursor, batchSize, recorder)
		if err != nil {
			return err
		}
		for i := range items {
			processed++
			start := time.Now()
			payload, err := exportDownloadPayloadFromSession(&items[i])
			recordDataShareExportDuration(recorder, DataShareExportDurationPartRedactRecheck, time.Since(start))
			if err != nil {
				if errors.Is(err, ErrDataShareExportPayloadInvalid) && (filters.SelectAll || len(filters.IDs) != 1) {
					slog.Warn("data sharing: skip session failed export recheck",
						"trajectory_id", items[i].TrajectoryID,
						"session_id", items[i].SessionID,
						"quality_status", items[i].QualityStatus,
						"error", err,
					)
					if progress != nil {
						progress(processed, total)
					}
					continue
				}
				return err
			}
			start = time.Now()
			line, err := json.Marshal(payload)
			recordDataShareExportDuration(recorder, DataShareExportDurationPartJSONMarshal, time.Since(start))
			if err != nil {
				return err
			}
			start = time.Now()
			if _, err := w.Write(append(line, '\n')); err != nil {
				recordDataShareExportDuration(recorder, DataShareExportDurationPartWriteCompress, time.Since(start))
				return err
			}
			recordDataShareExportDuration(recorder, DataShareExportDurationPartWriteCompress, time.Since(start))
			if progress != nil {
				progress(processed, total)
			}
		}
		cursor = nextCursor
		if len(items) == 0 || cursor == nil || len(items) < batchSize || (total > 0 && processed >= total) {
			if progress != nil {
				progress(processed, total)
			}
			return nil
		}
	}
}

func recordDataShareExportDuration(recorder DataShareExportDurationRecorder, part DataShareExportDurationPartKey, duration time.Duration) {
	if recorder == nil {
		return
	}
	recorder.Observe(part, duration)
}

func normalizeDataShareExportBatchSize(size int) int {
	return NormalizeDataShareExportBatchSize(size)
}

// NormalizeDataShareExportBatchSize 归一化预生成导出批次大小，避免单批读取过大。
func NormalizeDataShareExportBatchSize(size int) int {
	if size <= 0 {
		size = defaultDataShareExportBatchSize
	}
	if size < minDataShareExportBatchSize {
		return minDataShareExportBatchSize
	}
	if size > maxDataShareExportBatchSize {
		return maxDataShareExportBatchSize
	}
	return size
}

func validateDataShareExportTicketRequest(req DataShareExportTicketRequest) error {
	switch req.Scope {
	case DataShareExportScopeUser:
		if req.UserID <= 0 {
			return ErrDataShareExportTicketInvalid
		}
		if req.Filters.UserID != 0 && req.Filters.UserID != req.UserID {
			return ErrDataShareExportTicketForbidden
		}
	case DataShareExportScopeAdmin:
	default:
		return ErrDataShareExportTicketInvalid
	}
	if req.Filters.SelectAll {
		return nil
	}
	if len(req.Filters.IDs) == 0 {
		return ErrDataShareExportTicketInvalid
	}
	return nil
}

func (s *DataSharingService) exportTicketSigningKey(ctx context.Context) ([]byte, error) {
	if s == nil || s.settingRepo == nil {
		return []byte("tokenrouter-data-sharing-export-ticket-dev-key"), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyDataSharingExportTicketKey)
	if err == nil && strings.TrimSpace(raw) != "" {
		return []byte(strings.TrimSpace(raw)), nil
	}
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	if err := s.settingRepo.Set(ctx, SettingKeyDataSharingExportTicketKey, secret); err != nil {
		return nil, err
	}
	return []byte(secret), nil
}

func signDataShareExportTicket(claims DataShareExportTicketClaims, key []byte) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + signDataShareExportTicketPayload(encodedPayload, key), nil
}

func parseDataShareExportTicket(token string, key []byte) (*DataShareExportTicketClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrDataShareExportTicketInvalid
	}
	expected := signDataShareExportTicketPayload(parts[0], key)
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return nil, ErrDataShareExportTicketInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrDataShareExportTicketInvalid
	}
	var claims DataShareExportTicketClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrDataShareExportTicketInvalid
	}
	return &claims, nil
}

func signDataShareExportTicketPayload(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func dataShareExportDownloadURL(scope DataShareExportScope, token string) string {
	if scope == DataShareExportScopeAdmin {
		return "/api/v1/admin/data-sharing/export/download?ticket=" + token
	}
	return "/api/v1/data-sharing/export/download?ticket=" + token
}

func normalizeDataShareExportEncoding(encoding DataShareExportEncoding) DataShareExportEncoding {
	switch encoding {
	case DataShareExportEncodingJSON:
		return DataShareExportEncodingJSON
	case DataShareExportEncodingJSONL:
		return DataShareExportEncodingJSONL
	default:
		return DataShareExportEncodingZstd
	}
}

func normalizeDataShareExportFilename(filename string, encoding DataShareExportEncoding) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "data-sharing-" + time.Now().Format("20060102-150405")
	}
	filename = strings.TrimSuffix(filename, ".jsonl.zst")
	filename = strings.TrimSuffix(filename, ".jsonl")
	filename = strings.TrimSuffix(filename, ".json")
	filename = strings.TrimSuffix(filename, ".zst")
	filename = strings.NewReplacer("/", "-", "\\", "-", "\x00", "").Replace(filename)
	switch normalizeDataShareExportEncoding(encoding) {
	case DataShareExportEncodingJSON:
		return filename + ".json"
	case DataShareExportEncodingJSONL:
		return filename + ".jsonl"
	default:
		return filename + ".jsonl.zst"
	}
}
