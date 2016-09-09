package minfs

import (
	"time"

	"bazil.org/fuse"
)

func (mfs *MinFS) Unlock(path string) error {
	mfs.m.Lock()
	defer mfs.m.Unlock()

	delete(mfs.locks, path)

	return nil
}

func (mfs *MinFS) Lock(path string) error {
	mfs.m.Lock()
	defer mfs.m.Unlock()

	mfs.locks[path] = true
	return nil
}

// IsLocked returns if the path is currently locked
func (mfs *MinFS) IsLocked(path string) bool {
	mfs.m.Lock()
	defer mfs.m.Unlock()

	_, ok := mfs.locks[path]
	return ok
}

// wait for the file lock to be unlocked
func (mfs *MinFS) wait(path string) error {
	// todo(nl5887): should we add mutex here? We cannot use mfs.m mutex,
	// because that will create deadlock

	// check if the file is locked, and wait for max 5 seconds for the file to be
	// acquired
	for i := 0; ; /* retries */ i++ {
		if !mfs.IsLocked(path) {
			break
		}

		if i > 25 /* max number of retries */ {
			return fuse.EPERM
		}

		time.Sleep(time.Millisecond * 200)
	}

	return nil
}
