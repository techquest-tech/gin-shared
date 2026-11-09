package core

import (
	"testing"

	"github.com/spf13/viper"
)

// TestGlobalViperEnvOverrideEnabledAtInit 验证全局单例的 AutomaticEnv + EnvKeyReplacer
// 在包加载期（core.init()）就已生效，无需等待 InitConfig / InitLogger。
//
// 这是对「app.cfg 解密成功后 InitConfig 提前 return 跳过 AutomaticEnv、导致加密部署下
// env 覆盖失效」修复的回归保护：即便后续 InitConfig 因加密路径提前返回，env 覆盖也应可用。
func TestGlobalViperEnvOverrideEnabledAtInit(t *testing.T) {
	const key = "core.test.global.viper.env.override"
	const want = "from-env"

	t.Setenv("CORE_TEST_GLOBAL_VIPER_ENV_OVERRIDE", want)
	if got := viper.GetString(key); got != want {
		t.Fatalf("AutomaticEnv not enabled at package init: got=%q want=%q", got, want)
	}
}
