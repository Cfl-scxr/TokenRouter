package schema

import (
	"github.com/TokenFlux/TokenRouter/ent/schema/mixins"
	"github.com/TokenFlux/TokenRouter/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CreativeRun 定义创作台异步任务的数据结构。
//
// 隐私红线：这张表只保存任务元数据与计费快照；
// 图片字节、mask、prompt 明文和 provider 响应只允许存于临时 Redis 存储，
// prompt 仅以不可逆 sha256（prompt_hash）落库。
type CreativeRun struct {
	ent.Schema
}

func (CreativeRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "creative_runs"},
	}
}

func (CreativeRun) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CreativeRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("run_id").MaxLen(64).Immutable(),
		field.Int64("user_id"),
		field.Int64("group_id"),
		// api_key_id 指向创作台隐藏执行 Key（managed_by = 'creative_studio'）。
		field.Int64("api_key_id"),
		// account_id 由 worker 执行阶段回填。
		field.Int64("account_id").Optional().Nillable(),
		field.String("model").MaxLen(128),
		// requested_model 记录客户端提交值，model 记录计费/路由模型。
		field.String("requested_model").MaxLen(128).Default(""),
		field.String("operation").MaxLen(16),
		field.Int("requested_output_count").Default(1),
		field.String("image_size").MaxLen(16).Default("1K"),
		field.String("aspect_ratio").MaxLen(16).Default(""),
		field.String("response_mime_type").MaxLen(64).Default("image/png"),
		// prompt_hash 是 prompt 明文的 sha256，禁止保存 prompt 本体。
		field.String("prompt_hash").MaxLen(64),
		// request_fingerprint 用于幂等，sha256(canonical JSON)，同样不可逆。
		field.String("request_fingerprint").MaxLen(64),
		field.String("idempotency_key").Optional().Nillable().MaxLen(255),
		field.String("status").MaxLen(20).Default("queued"),
		field.Float("estimated_cost").SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Default(0),
		field.Float("hold_amount").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("actual_cost").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		// 计费预占快照，仿 batch_image_jobs：先占订阅、再冻结余额。
		field.Float("balance_hold_amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Default(0),
		field.JSON("subscription_hold_allocations", []domain.BillingAllocation{}).
			Default(func() []domain.BillingAllocation { return []domain.BillingAllocation{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		// 定价快照：基础单价与订阅/余额来源倍率。
		field.Float("base_unit_price").SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Default(0),
		field.Float("subscription_rate_multiplier").SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Default(1),
		field.Float("balance_rate_multiplier").SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Default(1),
		field.Bool("plan_group_rate_multiplier_enabled").Default(true),
		field.String("error_code").Optional().Nillable().MaxLen(128),
		field.String("error_message").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int("attempt_count").Default(0),
		// version 乐观锁：状态转换时 +1。
		field.Int64("version").Default(1),
		field.Time("started_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("completed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("cancelled_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CreativeRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_id").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("status").Annotations(entsql.IndexWhere("status IN ('queued', 'running')")),
		index.Fields("request_fingerprint").Unique(),
		index.Fields("user_id", "idempotency_key").
			Unique().
			Annotations(entsql.IndexWhere("idempotency_key IS NOT NULL AND idempotency_key <> ''")),
	}
}
