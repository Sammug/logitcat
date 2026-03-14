// Package daemon manages the logitcat runtime directory, PID file, and process daemonization.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// RuntimeDir returns (and creates) ~/.logitcat — the daemon's home directory.
func RuntimeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	dir := filepath.Join(home, ".logitcat")
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

func PIDFile() string    { return filepath.Join(RuntimeDir(), "logitcat.pid") }
func SocketPath() string { return filepath.Join(RuntimeDir(), "logitcat.sock") }
func LogFile() string    { return filepath.Join(RuntimeDir(), "logitcat.log") }

// WritePID writes the current process PID to the PID file.
func WritePID() error {
	return os.WriteFile(PIDFile(), []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// ReadPID reads the stored PID; returns 0 if the file does not exist.
func ReadPID() int {
	data, err := os.ReadFile(PIDFile())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

// IsRunning returns true if a logitcat daemon is currently running.
func IsRunning() (bool, int) {
	pid := ReadPID()
	if pid == 0 {
		return false, 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	// Signal 0 tests process existence without sending a real signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		_ = RemovePID
		return false, 0
	}
	return true, pid
}

// Start re-launches the current executable with the provided args in a detached
// background session, redirecting its output to the daemon log file.
func Start(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve executable: %w", err)
	}

	logFile, err := os.OpenFile(LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}

	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from terminal

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	fmt.Printf("logitcat started (PID %d)\n", cmd.Process.Pid)
	fmt.Printf("Logs: %s\n", LogFile())
	return nil
}

func RemovePID() error { return os.Remove(PIDFile()) }
