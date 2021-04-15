// Copyright (c) 2021 MinIO, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package minfs

import (
	"time"

	"bazil.org/fuse"
)

// Unlock - unlock the lock at path.
func (mfs *MinFS) Unlock(path string) error {
	mfs.m.Lock()
	defer mfs.m.Unlock()

	delete(mfs.locks, path)

	return nil
}

// Lock - acquires a lock at path.
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
