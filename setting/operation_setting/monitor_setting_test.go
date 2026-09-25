package operation_setting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMonitorSetting_ChannelTestEnabledEnvOverridesEnabledConfig(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "false")
	t.Setenv("CHANNEL_TEST_FREQUENCY", "5")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 20,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.False(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(5), setting.AutoTestChannelMinutes)
}

func TestGetMonitorSetting_ChannelTestEnabledEnvCanEnableDisabledConfig(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "true")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 12,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.True(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(12), setting.AutoTestChannelMinutes)
}

func TestGetMonitorSettingPreservesAutoBanOnlyMode(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "")
	t.Setenv("CHANNEL_TEST_FREQUENCY", "")
	monitorSetting = MonitorSetting{ChannelTestMode: ChannelTestModeAutoBanOnly}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.Equal(t, ChannelTestModeAutoBanOnly, setting.ChannelTestMode)
}

func TestGetMonitorSettingNormalizesChannelTestConcurrency(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	tests := []struct {
		name        string
		concurrency int
		want        int
	}{
		{name: "missing uses safe default", concurrency: 0, want: DefaultChannelTestConcurrency},
		{name: "configured value is preserved", concurrency: 8, want: 8},
		{name: "oversized value is capped", concurrency: MaxChannelTestConcurrency + 1, want: MaxChannelTestConcurrency},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			monitorSetting = MonitorSetting{ChannelTestConcurrency: test.concurrency}

			setting := GetMonitorSetting()

			require.NotNil(t, setting)
			assert.Equal(t, test.want, setting.ChannelTestConcurrency)
		})
	}
}

func TestValidateChannelTestConcurrency(t *testing.T) {
	require.NoError(t, ValidateChannelTestConcurrency("1"))
	require.NoError(t, ValidateChannelTestConcurrency("32"))
	assert.Error(t, ValidateChannelTestConcurrency("0"))
	assert.Error(t, ValidateChannelTestConcurrency("33"))
	assert.Error(t, ValidateChannelTestConcurrency("1.5"))
}

// TestGetChannelHealthSettingFallsBack T2-1：channel_health getter 非法值独立回退默认，
// 默认值 = 历史代码常量（行为零变化）。
func TestGetChannelHealthSettingFallsBack(t *testing.T) {
	orig := channelHealthSetting
	t.Cleanup(func() { channelHealthSetting = orig })

	channelHealthSetting = ChannelHealthSetting{}
	assert.Equal(t, 3600*time.Second, GetChannelHealthWindowTTL())
	assert.Equal(t, 256, GetChannelHealthRingSize())
	assert.Equal(t, 70, GetChannelHealthSuccessWeight())
	best, worst := GetChannelHealthLatencyBounds()
	assert.Equal(t, 1500*time.Millisecond, best)
	assert.Equal(t, 10*time.Second, worst)
	assert.Equal(t, 0, GetChannelHealthMinScore())

	channelHealthSetting = ChannelHealthSetting{WindowSeconds: 7200, RingSize: 512, SuccessWeight: 80, LatencyBestMs: 2000, LatencyWorstMs: 20000, MinScore: 50}
	assert.Equal(t, 2*time.Hour, GetChannelHealthWindowTTL())
	assert.Equal(t, 512, GetChannelHealthRingSize())
	assert.Equal(t, 80, GetChannelHealthSuccessWeight())
	best, worst = GetChannelHealthLatencyBounds()
	assert.Equal(t, 2*time.Second, best)
	assert.Equal(t, 20*time.Second, worst)
	assert.Equal(t, 50, GetChannelHealthMinScore())

	// 非法值边界
	channelHealthSetting = ChannelHealthSetting{RingSize: 99999, SuccessWeight: 101, MinScore: -1}
	assert.Equal(t, 4096, GetChannelHealthRingSize())
	assert.Equal(t, 70, GetChannelHealthSuccessWeight())
	assert.Equal(t, 0, GetChannelHealthMinScore())
}
