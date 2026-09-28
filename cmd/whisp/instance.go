package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func lockPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "whisp.lock")
}

var errRunning = errors.New("whisp is already running")

// acquireLock takes the single-instance lock and records our PID in it.
// The returned file must stay open for the lifetime of the process.
func acquireLock() (*os.File, error) {
	f, err := os.OpenFile(lockPath(), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errRunning
	}
	f.Truncate(0)
	f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	return f, nil
}

// runningPID returns the PID of a running instance, or 0.
func runningPID() int {
	f, err := os.Open(lockPath())
	if err != nil {
		return 0
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return 0 // nobody holds it
	}
	b, _ := os.ReadFile(lockPath())
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid
}

func stopRunning() error {
	pid := runningPID()
	if pid == 0 {
		return errors.New("whisp is not running")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		if runningPID() == 0 {
			fmt.Println("whisp stopped")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("whisp (pid %d) did not exit", pid)
}

// daemonize re-executes whisp detached from the terminal, logging to logPath.
func daemonize(logPath string) error {
	// The log contains dictated text: keep it private, like the transcripts.
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := append([]string{exe, "--foreground"}, os.Args[1:]...)
	proc, err := os.StartProcess(exe, args, &os.ProcAttr{
		Files: []*os.File{nil, logf, logf},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		return err
	}
	fmt.Printf("whisp started (pid %d), log: %s\n", proc.Pid, logPath)
	return proc.Release()
}
