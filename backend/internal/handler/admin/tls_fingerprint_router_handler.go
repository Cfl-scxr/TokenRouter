package admin

import (
	"strconv"

	"github.com/TokenFlux/TokenRouter/internal/model"
	"github.com/TokenFlux/TokenRouter/internal/pkg/response"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/gin-gonic/gin"
)

// TLSFingerprintRouterHandler 处理 TLS 路由器的 HTTP 请求。
type TLSFingerprintRouterHandler struct {
	service *service.TLSFingerprintRouterService
}

// NewTLSFingerprintRouterHandler 创建 TLS 路由器处理器。
func NewTLSFingerprintRouterHandler(service *service.TLSFingerprintRouterService) *TLSFingerprintRouterHandler {
	return &TLSFingerprintRouterHandler{service: service}
}

// CreateTLSFingerprintRouterRequest 创建 TLS 路由器请求。
type CreateTLSFingerprintRouterRequest struct {
	Name        string                           `json:"name" binding:"required"`
	Description *string                          `json:"description"`
	Enabled     *bool                            `json:"enabled"`
	Rules       []model.TLSFingerprintRouterRule `json:"rules"`
}

// UpdateTLSFingerprintRouterRequest 更新 TLS 路由器请求。
type UpdateTLSFingerprintRouterRequest struct {
	Name        *string                          `json:"name"`
	Description *string                          `json:"description"`
	Enabled     *bool                            `json:"enabled"`
	Rules       []model.TLSFingerprintRouterRule `json:"rules"`
}

// List 获取所有 TLS 路由器。
// GET /api/v1/admin/tls-fingerprint-routers
func (h *TLSFingerprintRouterHandler) List(c *gin.Context) {
	routers, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, routers)
}

// GetByID 根据 ID 获取 TLS 路由器。
// GET /api/v1/admin/tls-fingerprint-routers/:id
func (h *TLSFingerprintRouterHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid router ID")
		return
	}
	router, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if router == nil {
		response.NotFound(c, "Router not found")
		return
	}
	response.Success(c, router)
}

// Create 创建 TLS 路由器。
// POST /api/v1/admin/tls-fingerprint-routers
func (h *TLSFingerprintRouterHandler) Create(c *gin.Context) {
	var req CreateTLSFingerprintRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	router := &model.TLSFingerprintRouter{
		Name:        req.Name,
		Description: req.Description,
		Enabled:     true,
		Rules:       req.Rules,
	}
	if req.Enabled != nil {
		router.Enabled = *req.Enabled
	}
	if router.Rules == nil {
		router.Rules = []model.TLSFingerprintRouterRule{}
	}
	created, err := h.service.Create(c.Request.Context(), router)
	if err != nil {
		if _, ok := err.(*model.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, created)
}

// Update 更新 TLS 路由器。
// PUT /api/v1/admin/tls-fingerprint-routers/:id
func (h *TLSFingerprintRouterHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid router ID")
		return
	}
	var req UpdateTLSFingerprintRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if existing == nil {
		response.NotFound(c, "Router not found")
		return
	}
	router := &model.TLSFingerprintRouter{
		ID:          id,
		Name:        existing.Name,
		Description: existing.Description,
		Enabled:     existing.Enabled,
		Rules:       existing.Rules,
	}
	if req.Name != nil {
		router.Name = *req.Name
	}
	if req.Description != nil {
		router.Description = req.Description
	}
	if req.Enabled != nil {
		router.Enabled = *req.Enabled
	}
	if req.Rules != nil {
		router.Rules = req.Rules
	}
	if router.Rules == nil {
		router.Rules = []model.TLSFingerprintRouterRule{}
	}
	updated, err := h.service.Update(c.Request.Context(), router)
	if err != nil {
		if _, ok := err.(*model.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

// Delete 删除 TLS 路由器。
// DELETE /api/v1/admin/tls-fingerprint-routers/:id
func (h *TLSFingerprintRouterHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid router ID")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Router deleted successfully"})
}
