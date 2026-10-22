//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type heldLock struct{ file *os.File }

func lockWorker(runtimeDir string) (*heldLock, error) {
	file, err := os.OpenFile(filepath.Join(runtimeDir, "managed-worker.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("managed worker lain sudah memakai runtime ini; hentikan worker duplikat terlebih dahulu")
	}
	return &heldLock{file: file}, nil
}

func unlockWorker(lock *heldLock) {
	if lock == nil || lock.file == nil {
		return
	}
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	_ = lock.file.Close()
}
