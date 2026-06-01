package repository

import (
	"context"

	"github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/tlsfingerprintrouter"
	"github.com/TokenFlux/TokenRouter/internal/model"
	"github.com/TokenFlux/TokenRouter/internal/service"
)

type tlsFingerprintRouterRepository struct {
	client *ent.Client
}

// NewTLSFingerprintRouterRepository 创建 TLS 路由器仓库。
func NewTLSFingerprintRouterRepository(client *ent.Client) service.TLSFingerprintRouterRepository {
	return &tlsFingerprintRouterRepository{client: client}
}

// List 获取所有 TLS 路由器。
func (r *tlsFingerprintRouterRepository) List(ctx context.Context) ([]*model.TLSFingerprintRouter, error) {
	routers, err := r.client.TLSFingerprintRouter.Query().
		Order(ent.Asc(tlsfingerprintrouter.FieldName)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.TLSFingerprintRouter, len(routers))
	for i, router := range routers {
		result[i] = r.toModel(router)
	}
	return result, nil
}

// GetByID 根据 ID 获取 TLS 路由器。
func (r *tlsFingerprintRouterRepository) GetByID(ctx context.Context, id int64) (*model.TLSFingerprintRouter, error) {
	router, err := r.client.TLSFingerprintRouter.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return r.toModel(router), nil
}

// Create 创建 TLS 路由器。
func (r *tlsFingerprintRouterRepository) Create(ctx context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	builder := r.client.TLSFingerprintRouter.Create().
		SetName(router.Name).
		SetEnabled(router.Enabled).
		SetRules(router.Rules)
	if router.Description != nil {
		builder.SetDescription(*router.Description)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return r.toModel(created), nil
}

// Update 更新 TLS 路由器。
func (r *tlsFingerprintRouterRepository) Update(ctx context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	builder := r.client.TLSFingerprintRouter.UpdateOneID(router.ID).
		SetName(router.Name).
		SetEnabled(router.Enabled).
		SetRules(router.Rules)
	if router.Description != nil {
		builder.SetDescription(*router.Description)
	} else {
		builder.ClearDescription()
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return r.toModel(updated), nil
}

// Delete 删除 TLS 路由器。
func (r *tlsFingerprintRouterRepository) Delete(ctx context.Context, id int64) error {
	return r.client.TLSFingerprintRouter.DeleteOneID(id).Exec(ctx)
}

func (r *tlsFingerprintRouterRepository) toModel(e *ent.TLSFingerprintRouter) *model.TLSFingerprintRouter {
	router := &model.TLSFingerprintRouter{
		ID:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		Enabled:     e.Enabled,
		Rules:       e.Rules,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
	if router.Rules == nil {
		router.Rules = []model.TLSFingerprintRouterRule{}
	}
	return router
}
