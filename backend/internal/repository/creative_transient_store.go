package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/redis/go-redis/v9"
)

// 创作台临时存储 Redis 键前缀。
// 输入载荷、源图字节、mask 与输出图片本体只保存在这些键下，TTL 到期即失效。
const (
	defaultCreativePayloadKeyPrefix = "creative:payload:"
	defaultCreativeInputKeyPrefix   = "creative:input:"
	defaultCreativeMaskKeyPrefix    = "creative:mask:"
	defaultCreativeOutputKeyPrefix  = "creative:output:"
)

// creativeTransientStore 基于 Redis 的创作台临时存储。
// Redis 不可用时所有写操作返回明确错误，由服务层 fail-close 拒绝新任务。
type creativeTransientStore struct {
	rdb           *redis.Client
	payloadPrefix string
	inputPrefix   string
	maskPrefix    string
	outputPrefix  string
	defaultTTL    time.Duration
}

// NewCreativeTransientStore 创建创作台临时存储。
func NewCreativeTransientStore(rdb *redis.Client, cfg *config.Config) service.CreativeTransientStore {
	store := &creativeTransientStore{
		rdb:           rdb,
		payloadPrefix: defaultCreativePayloadKeyPrefix,
		inputPrefix:   defaultCreativeInputKeyPrefix,
		maskPrefix:    defaultCreativeMaskKeyPrefix,
		outputPrefix:  defaultCreativeOutputKeyPrefix,
		defaultTTL:    30 * time.Minute,
	}
	if cfg != nil && cfg.Creative.TransientTTLSeconds > 0 {
		store.defaultTTL = time.Duration(cfg.Creative.TransientTTLSeconds) * time.Second
	}
	return store
}

func (s *creativeTransientStore) SavePayload(ctx context.Context, runID string, payload *service.CreativeRunPayload) error {
	if s.rdb == nil {
		return errors.New("creative transient store redis client is nil")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	key := s.payloadPrefix + runID
	// 写前删除再设置，保证重复保存同一任务时 TTL 与内容一致（幂等覆盖）。
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, key)
	pipe.Set(ctx, key, body, s.defaultTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *creativeTransientStore) LoadPayload(ctx context.Context, runID string) (*service.CreativeRunPayload, error) {
	if s.rdb == nil {
		return nil, errors.New("creative transient store redis client is nil")
	}
	body, err := s.rdb.Get(ctx, s.payloadPrefix+runID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrCreativeTransientFailed
	}
	if err != nil {
		return nil, err
	}
	payload := &service.CreativeRunPayload{}
	if err := json.Unmarshal(body, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *creativeTransientStore) SaveInput(ctx context.Context, runID string, idx int, data []byte) error {
	if s.rdb == nil {
		return errors.New("creative transient store redis client is nil")
	}
	if len(data) == 0 {
		return errors.New("creative input is empty")
	}
	return s.rdb.Set(ctx, s.inputKey(runID, idx), data, s.defaultTTL).Err()
}

func (s *creativeTransientStore) LoadInputs(ctx context.Context, runID string, count int) ([][]byte, error) {
	if s.rdb == nil {
		return nil, errors.New("creative transient store redis client is nil")
	}
	if count <= 0 {
		return nil, nil
	}
	keys := make([]string, 0, count)
	for idx := 0; idx < count; idx++ {
		keys = append(keys, s.inputKey(runID, idx))
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, count)
	for idx, value := range values {
		raw, ok := value.(string)
		if !ok || raw == "" {
			return nil, fmt.Errorf("creative input %d for run %s is missing or expired", idx, runID)
		}
		out = append(out, []byte(raw))
	}
	return out, nil
}

func (s *creativeTransientStore) SaveMask(ctx context.Context, runID string, data []byte) error {
	if s.rdb == nil {
		return errors.New("creative transient store redis client is nil")
	}
	if len(data) == 0 {
		return errors.New("creative mask is empty")
	}
	return s.rdb.Set(ctx, s.maskPrefix+runID, data, s.defaultTTL).Err()
}

func (s *creativeTransientStore) LoadMask(ctx context.Context, runID string) ([]byte, error) {
	if s.rdb == nil {
		return nil, errors.New("creative transient store redis client is nil")
	}
	data, err := s.rdb.Get(ctx, s.maskPrefix+runID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrCreativeTransientFailed
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *creativeTransientStore) SaveOutput(ctx context.Context, runID string, index int, data []byte, ttl time.Duration) error {
	if s.rdb == nil {
		return errors.New("creative transient store redis client is nil")
	}
	if len(data) == 0 {
		return errors.New("creative output is empty")
	}
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	return s.rdb.Set(ctx, s.outputKey(runID, index), data, ttl).Err()
}

func (s *creativeTransientStore) LoadOutput(ctx context.Context, runID string, index int) ([]byte, error) {
	if s.rdb == nil {
		return nil, errors.New("creative transient store redis client is nil")
	}
	data, err := s.rdb.Get(ctx, s.outputKey(runID, index)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrCreativeTransientFailed
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, service.ErrCreativeTransientFailed
	}
	return data, nil
}

func (s *creativeTransientStore) DeleteOutput(ctx context.Context, runID string, index int) error {
	if s.rdb == nil {
		return errors.New("creative transient store redis client is nil")
	}
	// DEL 对不存在的键天然幂等。
	return s.rdb.Del(ctx, s.outputKey(runID, index)).Err()
}

// DeleteRunTransient 删除任务全部临时键；inputCount/outputCount 未知时传 0 会退化为通配扫描。
func (s *creativeTransientStore) DeleteRunTransient(ctx context.Context, runID string, inputCount, outputCount int) error {
	if s.rdb == nil {
		return errors.New("creative transient store redis client is nil")
	}
	keys := []string{
		s.payloadPrefix + runID,
		s.maskPrefix + runID,
	}
	for idx := 0; idx < inputCount; idx++ {
		keys = append(keys, s.inputKey(runID, idx))
	}
	for index := 0; index < outputCount; index++ {
		keys = append(keys, s.outputKey(runID, index))
	}
	// 计数未知（如取消路径）时按前缀扫描补齐，保证清理完整。
	if inputCount <= 0 {
		if scanned, err := s.rdb.Keys(ctx, s.inputPrefix+runID+":*").Result(); err == nil {
			keys = append(keys, scanned...)
		}
	}
	if outputCount <= 0 {
		if scanned, err := s.rdb.Keys(ctx, s.outputPrefix+runID+":*").Result(); err == nil {
			keys = append(keys, scanned...)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return s.rdb.Del(ctx, keys...).Err()
}

func (s *creativeTransientStore) inputKey(runID string, idx int) string {
	return fmt.Sprintf("%s%s:%d", s.inputPrefix, runID, idx)
}

func (s *creativeTransientStore) outputKey(runID string, index int) string {
	return fmt.Sprintf("%s%s:%d", s.outputPrefix, runID, index)
}

var _ service.CreativeTransientStore = (*creativeTransientStore)(nil)
