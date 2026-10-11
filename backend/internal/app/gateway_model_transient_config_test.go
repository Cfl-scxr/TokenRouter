package app

import (
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
)

// 配置关闭瞬态冷却时，反馈和选路共用的状态仍记录失败，却不阻止重连。
func TestSelectionModelTransientCooldownConfig(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Gateway.ModelTransientCooldownDisabled = disabled
		state := provideSelectionModelTransient(cfg)
		now := time.Now()
		for i := 0; i < 4; i++ {
			state.RecordFailure(12, "synthetic", now)
		}
		if state.IsBlocked(12, "synthetic", now) == disabled {
			t.Fatalf("cooldown disabled=%v was not applied", disabled)
		}
	}
}
