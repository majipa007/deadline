package task

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// lockAttempts and lockDelay bound how long a writer waits for a competing
// session: about two seconds total. Brief headless writes never hold it
// longer than milliseconds; a stuck holder fails the waiter with a retry
// message instead of hanging an agent forever.
const (
	lockAttempts = 40
	lockDelay    = 50 * time.Millisecond
)

// BoardLock is an advisory exclusive lock on one board file, so two agents
// (or an agent plus an open TUI) cannot load-modify-save over each other
// and silently drop tasks. Reads take no lock: Save's atomic rename means
// readers never see a half-written board. One empty file per board lives in
// the temp dir; the OS reclaims it with the rest of tmp. Best-effort on
// filesystems without flock (e.g. some NFS mounts), where writers fall
// back to the old last-writer-wins behaviour.
type BoardLock struct {
	f *os.File
}

// LockBoard takes the lock for boardPath, via a lock file in the system
// temp dir keyed by the board's absolute path — never beside the board
// itself, so it litters neither projects nor config dirs, and needs no
// gitignore. Hold it across the whole load-modify-save and Close it
// afterwards.
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
