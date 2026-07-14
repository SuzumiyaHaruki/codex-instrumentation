package commands

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cfg "github.com/cometbft/cometbft/config"
)

type controllerMediationCommandNode struct {
	mu      sync.Mutex
	running bool
	quit    chan struct{}
	fatal   error
	stopErr error
}

func (n *controllerMediationCommandNode) IsRunning() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.running
}
func (n *controllerMediationCommandNode) Quit() <-chan struct{} { return n.quit }
func (n *controllerMediationCommandNode) FatalError() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fatal
}

func (n *controllerMediationCommandNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		n.running = false
		close(n.quit)
	}
	return n.stopErr
}

func (n *controllerMediationCommandNode) fail(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.fatal = err
	n.running = false
	close(n.quit)
}

func TestControllerMediationRuntimeFatalExit(t *testing.T) {
	expected := errors.New("controller unavailable")
	node := &controllerMediationCommandNode{quit: make(chan struct{}), fatal: expected}
	close(node.quit)
	err := waitForControllerMediationNode(node, make(chan os.Signal))
	require.ErrorIs(t, err, expected)

	node = &controllerMediationCommandNode{running: true, quit: make(chan struct{})}
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	require.NoError(t, waitForControllerMediationNode(node, signals))
	require.False(t, node.running)

	node = &controllerMediationCommandNode{running: true, quit: make(chan struct{}), fatal: expected, stopErr: errors.New("already stopped")}
	signals = make(chan os.Signal, 1)
	signals <- os.Interrupt
	require.ErrorIs(t, waitForControllerMediationNode(node, signals), expected)
}

func TestControllerMediationIntegratedFailureNonZeroExit(t *testing.T) {
	expected := errors.New("controller/control-channel failure: selected submission returned HTTP 503")
	node := &controllerMediationCommandNode{running: true, quit: make(chan struct{})}
	go node.fail(expected)

	result := make(chan error, 1)
	go func() { result <- waitForControllerMediationNode(node, make(chan os.Signal)) }()
	select {
	case err := <-result:
		require.ErrorIs(t, err, expected)
		require.False(t, node.IsRunning())
	case <-time.After(time.Second):
		t.Fatal("runtime controller failure did not produce non-zero owner return")
	}
}

func TestControllerMediationTestnetConfig(t *testing.T) {
	oldPrefix, oldSuffix, oldStartingIP, oldHostnames := hostnamePrefix, hostnameSuffix, startingIPAddress, hostnames
	t.Cleanup(func() {
		hostnamePrefix, hostnameSuffix, startingIPAddress, hostnames = oldPrefix, oldSuffix, oldStartingIP, oldHostnames
	})
	hostnamePrefix, hostnameSuffix, startingIPAddress, hostnames = "replica", "", "", nil

	config := cfg.DefaultConfig()
	config.ExperimentalController.Enabled = true
	config.ExperimentalController.ControllerAddress = "controller:31000"
	config.ExperimentalController.CallbackListenAddress = "0.0.0.0:31001"
	config.ExperimentalController.CallbackAdvertiseAddress = "placeholder:31001"
	require.NoError(t, configureControllerMediationForTestnet(config, 3))
	require.Equal(t, 3, config.ExperimentalController.NodeID)
	require.Equal(t, "replica3:31001", config.ExperimentalController.CallbackAdvertiseAddress)
	require.Equal(t, "controller:31000", config.ExperimentalController.ControllerAddress)
	require.Equal(t, "0.0.0.0:31001", config.ExperimentalController.CallbackListenAddress)
}
