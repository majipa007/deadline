package task

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// lockAttempts and lockDelay cap the wait for a competing writer at ~2s, so a
// stuck holder fails with a retry message instead of hanging.
const (
	lockAttempts = 40
	lockDelay    = 50 * time.Millisecond
)

// BoardLock is an advisory exclusive lock on one board file, so concurrent
// writers cannot load-modify-save over each other. Reads take no lock (Save
// renames atomically). Best-effort on filesystems without flock.
type BoardLock struct {
	f *os.File
}

// LockBoard takes the lock for boardPath via a lock file in the temp dir
// keyed by its absolute path. Hold it across load-modify-save, then Close.
func LockBoard(boardPath string) (*BoardLock, error) {
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		abs = boardPath
	}
	sum := sha1.Sum([]byte(abs))
	lockPath := filepath.Join(os.TempDir(), fmt.Sprintf("gotodo-%x.lock", sum))
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock board: %w", err)
	}
	for i := 0; ; i++ {
		if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			return &BoardLock{f: f}, nil
		}
		if i >= lockAttempts {
			f.Close()
			return nil, fmt.Errorf("board is busy (another session holds it); wait a moment and retry")
		}
		time.Sleep(lockDelay)
	}
}

// Close releases the lock.
func (l *BoardLock) Close() error {
	defer l.f.Close()
	return unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
}
