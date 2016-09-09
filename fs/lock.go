/*
 * MinFS - fuse driver for Object Storage (C) 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

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
