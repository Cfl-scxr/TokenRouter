package handler

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/response"
	middleware2 "github.com/TokenFlux/TokenRouter/internal/server/middleware"
	"github.com/TokenFlux/TokenRouter/internal/service"

	"github.com/gin-gonic/gin"
)

// 创作台 multipart 单字段大小上限：与配置的单文件上限（默认 32 MiB）对齐并留出头部的余量。
const creativeMaxUploadPartSize = 40 << 20

// CreativeHandler 处理创作台用户侧请求。
type CreativeHandler struct {
	service *service.CreativePublicService
}

// NewCreativeHandler 创建创作台 handler。
func NewCreativeHandler(service *service.CreativePublicService) *CreativeHandler {
	return &CreativeHandler{service: service}
}

// ListModels 返回当前用户可用的分组 + 图片模型组合。
// GET /api/v1/creative/models
func (h *CreativeHandler) ListModels(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	got, err := h.service.ListModels(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 面板 envelope 的 data 直接放数组，避免再包一层 {data: [...]}。
	response.Success(c, got.Data)
}

// creativeCreateRunRequest 是创建任务 multipart 报文的解析结果。
type creativeCreateRunRequest struct {
	GroupID      int64
	Model        string
	Operation    string
	Prompt       string
	SourceImages []service.CreativeInputImage
	Mask         *service.CreativeInputImage
	ImageSize    string
	AspectRatio  string
	ResponseMIME string
	Quality      string
}

// CreateRun 解析 multipart/form-data 并创建创作台任务。
// POST /api/v1/creative/runs
func (h *CreativeHandler) CreateRun(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	req, err := parseCreativeCreateRunMultipart(c)
	if err != nil {
		response.ErrorFrom(c, service.ErrCreativeInvalidParams)
		return
	}
	got, err := h.service.CreateRun(c.Request.Context(), subject.UserID, service.CreateCreativeRunParamsPublic{
		GroupID:      req.GroupID,
		Model:        req.Model,
		Operation:    req.Operation,
		Prompt:       req.Prompt,
		SourceImages: req.SourceImages,
		Mask:         req.Mask,
		ImageSize:    req.ImageSize,
		AspectRatio:  req.AspectRatio,
		ResponseMIME: req.ResponseMIME,
		Quality:      req.Quality,
	}, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, got)
}

// parseCreativeCreateRunMultipart 手工解析 multipart 表单：
// 字段 group_id/model/operation/prompt/image_size/aspect_ratio/response_mime_type，
// 文件字段 source_images（多文件）与 mask（单文件）。只接受上传文件，不接受远程 URL。
func parseCreativeCreateRunMultipart(c *gin.Context) (*creativeCreateRunRequest, error) {
	contentType := c.GetHeader("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, http.ErrMissingBoundary
	}
	req := &creativeCreateRunRequest{}
	reader := multipart.NewReader(c.Request.Body, boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}
		fileName := strings.TrimSpace(part.FileName())
		if fileName != "" {
			data, readErr := io.ReadAll(io.LimitReader(part, creativeMaxUploadPartSize))
			_ = part.Close()
			if readErr != nil {
				return nil, readErr
			}
			partMIME := normalizeCreativeUploadMime(strings.TrimSpace(part.Header.Get("Content-Type")), data)
			switch {
			case name == "mask":
				if len(data) > 0 {
					req.Mask = &service.CreativeInputImage{Bytes: data, Mime: partMIME}
				}
			case name == "source_images" || name == "source_images[]" || strings.HasPrefix(name, "source_images["):
				if len(data) > 0 {
					req.SourceImages = append(req.SourceImages, service.CreativeInputImage{Bytes: data, Mime: partMIME})
				}
			}
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(part, 1<<20))
		_ = part.Close()
		if readErr != nil {
			return nil, readErr
		}
		value := strings.TrimSpace(string(data))
		switch name {
		case "group_id":
			groupID, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil || groupID <= 0 {
				return nil, service.ErrCreativeInvalidParams
			}
			req.GroupID = groupID
		case "model":
			req.Model = value
		case "operation":
			req.Operation = strings.ToLower(value)
		case "prompt":
			req.Prompt = value
		case "image_size":
			req.ImageSize = value
		case "aspect_ratio":
			req.AspectRatio = value
		case "response_mime_type":
			req.ResponseMIME = value
		case "quality":
			req.Quality = value
		}
	}
	if req.GroupID <= 0 {
		return nil, service.ErrCreativeInvalidParams
	}
	return req, nil
}

// normalizeCreativeUploadMime 归一化上传文件 MIME；缺失或非法时按字节魔数嗅探。
func normalizeCreativeUploadMime(headerMIME string, data []byte) string {
	switch strings.ToLower(strings.TrimSpace(headerMIME)) {
	case "image/png":
		return "image/png"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	}
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return "image/png"
	}
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return strings.ToLower(strings.TrimSpace(headerMIME))
}

// ListRuns 返回当前用户的任务列表。
// GET /api/v1/creative/runs
func (h *CreativeHandler) ListRuns(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	got, err := h.service.ListRuns(c.Request.Context(), subject.UserID, service.CreativeRunFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Limit:  limit,
		Offset: 0,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 面板 envelope 的 data 直接放数组，前端按数组或 {items,total} 宽容解析。
	response.Success(c, got.Data)
}

// GetRun 返回单个任务详情。
// GET /api/v1/creative/runs/:id
func (h *CreativeHandler) GetRun(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	got, err := h.service.GetRun(c.Request.Context(), subject.UserID, c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, got)
}

// GetOutputContent 返回临时输出图片字节；过期返回 410 风格错误。
// GET /api/v1/creative/runs/:id/outputs/:index/content
func (h *CreativeHandler) GetOutputContent(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	outputIndex, err := strconv.Atoi(c.Param("index"))
	if err != nil || outputIndex < 0 {
		response.ErrorFrom(c, service.ErrCreativeOutputNotFound)
		return
	}
	content, err := h.service.GetOutputContent(c.Request.Context(), subject.UserID, c.Param("id"), outputIndex)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, content.ContentType, content.Content)
}

// AckOutput 客户端确认接收后删除临时输出。
// POST /api/v1/creative/runs/:id/outputs/:index/ack
func (h *CreativeHandler) AckOutput(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	outputIndex, err := strconv.Atoi(c.Param("index"))
	if err != nil || outputIndex < 0 {
		response.ErrorFrom(c, service.ErrCreativeOutputNotFound)
		return
	}
	if err := h.service.AckOutput(c.Request.Context(), subject.UserID, c.Param("id"), outputIndex); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"acked": true})
}
