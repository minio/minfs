// This file is part of MinFS
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

/*
 * MinFS - fuse driver for Object Storage (C) 2016 MinIO, Inc.
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
	"net/url"
	"os"
	"path"
	"strings"

	minio "github.com/minio/minio-go/v6"
)

func (mfs *MinFS) startNotificationListener() error {
	events := []string{string(minio.ObjectCreatedAll), string(minio.ObjectRemovedAll)}

	// Start listening on all bucket events.
	eventsCh := mfs.api.ListenBucketNotification(mfs.config.bucket, "", "", events, mfs.listenerDoneCh)
	go func() {
		for {
			select {
			case notificationInfo := <-eventsCh:
				if notificationInfo.Err != nil {
					continue
				}

				// Start a writable transaction.
				tx, err := mfs.db.Begin(true)
				if err != nil {
					panic(err)
				}
				for _, record := range notificationInfo.Records {
					key, e := url.QueryUnescape(record.S3.Object.Key)
					if e != nil {
						mfs.log.Println("Error:", err)
						tx.Rollback()
						continue
					}

					dir, file := path.Split(key)

					var d *Dir
					if dir == "" {
						d = &Dir{
							dir: nil,

							mfs:  mfs,
							Mode: os.ModeDir | 0555,
							Path: "",
						}
					} else {
						rootDir, _ := mfs.Root()
						d = &Dir{
							dir:  rootDir.(*Dir),
							mfs:  mfs,
							Mode: 0770 | os.ModeDir,
							Path: dir,
							GID:  mfs.config.gid,
							UID:  mfs.config.uid,
						}
					}

					if strings.HasPrefix(record.EventName, "s3:ObjectCreated:") {
						if err = d.storeFile(d.bucket(tx), tx, file, minio.ObjectInfo{
							Key:  record.S3.Object.Key,
							Size: record.S3.Object.Size,
							ETag: record.S3.Object.ETag,
						}); err != nil {
							tx.Rollback()
							mfs.log.Println("Error:", err)
							continue
						}
					}

				}

				// Commit the transaction and check for error.
				if err := tx.Commit(); err != nil {
					tx.Rollback()
					panic(err)
				}
			case <-mfs.listenerDoneCh:
				return
			}
		}
	}()
	return nil
}

func (mfs *MinFS) stopNotificationListener() error {
	close(mfs.listenerDoneCh)
	return nil
}
