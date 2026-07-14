package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cometbft/cometbft/config"
)

func enabledControllerConfig() *config.ControllerMediationConfig {
	cfg := config.DefaultControllerMediationConfig()
	cfg.Enabled = true
	cfg.ControllerAddress = "127.0.0.1:31000"
	cfg.CallbackListenAddress = "127.0.0.1:31001"
	cfg.CallbackAdvertiseAddress = "node-0:31001"
	return cfg
}

func TestControllerMediationConfigDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	require.NotNil(t, cfg.ExperimentalController)
	require.False(t, cfg.ExperimentalController.Enabled)
	require.NoError(t, cfg.ValidateBasic())
	require.NotSame(t, cfg.Instrumentation, cfg.ExperimentalController)
}

func TestControllerMediationConfigValidation(t *testing.T) {
	require.NoError(t, enabledControllerConfig().ValidateBasic())

	tests := map[string]func(*config.ControllerMediationConfig){
		"controller address":    func(c *config.ControllerMediationConfig) { c.ControllerAddress = "" },
		"listen address":        func(c *config.ControllerMediationConfig) { c.CallbackListenAddress = "bad" },
		"advertise address":     func(c *config.ControllerMediationConfig) { c.CallbackAdvertiseAddress = "" },
		"node id":               func(c *config.ControllerMediationConfig) { c.NodeID = -1 },
		"outbound items":        func(c *config.ControllerMediationConfig) { c.OutboundQueueCapacity = 0 },
		"outbound bytes":        func(c *config.ControllerMediationConfig) { c.OutboundQueueBytes = 1 },
		"callback items":        func(c *config.ControllerMediationConfig) { c.CallbackQueueCapacity = 0 },
		"callback bytes":        func(c *config.ControllerMediationConfig) { c.CallbackQueueBytes = 1 },
		"callback body":         func(c *config.ControllerMediationConfig) { c.CallbackMaxBodyBytes = 1 },
		"counter registry":      func(c *config.ControllerMediationConfig) { c.CounterRegistryCapacity = 0 },
		"registration attempts": func(c *config.ControllerMediationConfig) { c.RegistrationAttempts = 0 },
		"registration delay":    func(c *config.ControllerMediationConfig) { c.RegistrationRetryDelay = 3 * time.Second },
		"request timeout":       func(c *config.ControllerMediationConfig) { c.RequestTimeout = 0 },
		"send timeout":          func(c *config.ControllerMediationConfig) { c.SendTimeout = 11 * time.Second },
		"shutdown timeout":      func(c *config.ControllerMediationConfig) { c.ShutdownTimeout = 0 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := enabledControllerConfig()
			mutate(cfg)
			require.Error(t, cfg.ValidateBasic())
		})
	}

	disabled := &config.ControllerMediationConfig{}
	require.NoError(t, disabled.ValidateBasic())
}
