//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type heldLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func lockWorker(runtimeDir string) (*heldLock, error) {
	path := filepath.Join(runtimeDir, "managed-worker.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if info, statErr := file.Stat(); statErr != nil {
		file.Close()
		return nil, statErr
	} else if info.Size() == 0 {
		if _, writeErr := file.WriteAt([]byte{0}, 0); writeErr != nil {
			file.Close()
			return nil, writeErr
		}
	}
	lock := &heldLock{file: file}
	err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &lock.overlapped)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("managed worker lain sudah memakai runtime ini; hentikan worker duplikat terlebih dahulu")
	}
	return lock, nil
}

func unlockWorker(lock *heldLock) {
	if lock == nil || lock.file == nil {
		return
	}
	_ = windows.UnlockFileEx(windows.Handle(lock.file.Fd()), 0, 1, 0, &lock.overlapped)
	_ = lock.file.Close()
}
