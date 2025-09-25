package tor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/logger"
)

// Manager supervises a single tor process used as an upstream SOCKS proxy. It automatically restarts
// the process on unexpected termination and exposes helper methods to guarantee availability.
type Manager struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

var (
	instance     *Manager
	instanceOnce sync.Once
)

// GetManager returns the singleton tor manager instance.
func GetManager() *Manager {
	instanceOnce.Do(func() {
		instance = &Manager{}
	})
	return instance
}

// EnsureRunning starts the tor process if it is not already running.
func (m *Manager) EnsureRunning() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil && m.cmd.Process != nil {
		if m.cmd.ProcessState == nil {
			return nil
		}
	}

	return m.startLocked()
}

// startLocked assumes the mutex is held and starts a fresh tor process.
func (m *Manager) startLocked() error {
	torBinary := config.GetTorBinaryPath()
	if torBinary == "" {
		return errors.New("tor binary path not configured")
	}

	absDataDir := config.GetTorDataDir()
	if !filepath.IsAbs(absDataDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		absDataDir = filepath.Join(cwd, absDataDir)
	}

	if err := os.MkdirAll(absDataDir, 0o700); err != nil {
		return fmt.Errorf("create tor data dir: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	args := []string{
		"--RunAsDaemon", "0",
		"--ClientOnly", "1",
		"--SocksPort", fmt.Sprintf("%s:%d IsolateSOCKSAuth", config.GetTorSocksAddress(), config.GetTorSocksPort()),
		"--ControlPort", fmt.Sprintf("%d", config.GetTorControlPort()),
		"--DataDirectory", absDataDir,
		"--Log", "notice stdout",
	}

	cmd := exec.CommandContext(ctx, torBinary, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("pipe tor stdout: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("pipe tor stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start tor process: %w", err)
	}

	logger.Infof("Started tor upstream process (pid=%d) on %s:%d", cmd.Process.Pid, config.GetTorSocksAddress(), config.GetTorSocksPort())

	go m.pipeOutput(stdout, "tor")
	go m.pipeOutput(stderr, "tor")
	go m.monitor(ctx, cmd)

	m.cmd = cmd
	m.cancel = cancel

	return nil
}

// pipeOutput streams process output line by line into the shared logger.
func (m *Manager) pipeOutput(r io.Reader, prefix string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		logger.Debugf("%s: %s", prefix, scanner.Text())
	}
}

// monitor waits for the tor process to exit and attempts automatic restarts.
func (m *Manager) monitor(ctx context.Context, cmd *exec.Cmd) {
	err := cmd.Wait()
	if ctx.Err() == context.Canceled {
		logger.Debug("tor process stopped")
		return
	}

	if err != nil {
		logger.Warningf("tor process exited unexpectedly: %v", err)
	} else {
		logger.Warning("tor process exited unexpectedly")
	}

	m.mu.Lock()
	m.cmd = nil
	m.cancel = nil
	m.mu.Unlock()

	// Retry with exponential backoff
	for attempt := 0; attempt < 5; attempt++ {
		delay := time.Duration(attempt+1) * time.Second
		time.Sleep(delay)
		if err := m.EnsureRunning(); err == nil {
			return
		} else {
			logger.Warningf("failed to restart tor (attempt %d): %v", attempt+1, err)
		}
	}
}

// Stop terminates the tor process if it is running.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	m.cmd = nil
	m.cancel = nil
}
