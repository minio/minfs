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

import minio "github.com/minio/minio-go"

func (mfs *MinFS) startNotificationListener() error {
	return nil
	/*
		// try to set and listen for notifications
		// Fetch the bucket location.
		location, err := mfs.api.GetBucketLocation(mfs.config.bucket)
		if err != nil {
			return err
		}

		// Fetch any existing bucket notification on the bucket.
		bn, err := mfs.api.GetBucketNotification(mfs.config.bucket)
		if err != nil {
			return err
		}

		accountARN := minio.NewArn("minio", "sns", location, mfs.config.accountID, "listen")

		// If there are no SNS topics configured, configure the first one.
		shouldSetNotification := len(bn.TopicConfigs) == 0
		if !shouldSetNotification {
			// We found previously configure SNS topics, validate if current account-id is the same.
			// this will always set shouldSetNotification right?
			for _, topicConfig := range bn.TopicConfigs {
				if topicConfig.Topic == accountARN.String() {
					shouldSetNotification = false
					break
				}
			}
		}

		if shouldSetNotification {
			topicConfig := minio.NewNotificationConfig(accountARN)
			topicConfig.AddEvents(minio.ObjectCreatedAll, minio.ObjectRemovedAll)
			bn.AddTopic(topicConfig)

			if err := mfs.api.SetBucketNotification(mfs.config.bucket, bn); err != nil {
				return err
			}
		}

		doneCh := make(chan struct{})

		// todo(nl5887): reconnect on close
		eventsCh := mfs.api.ListenBucketNotification(mfs.config.bucket, accountARN, doneCh)
		go func() {
			for notificationInfo := range eventsCh {
				if notificationInfo.Err != nil {
					continue
				}

				// Start a writable transaction.
				tx, err := mfs.db.Begin(true)
				if err != nil {
					panic(err)
				}

				defer tx.Rollback()
				// todo(nl5887): defer not called in for each
				// todo(nl5887): how to ignore my own created events?
				// can we use eventsource?

				for _, record := range notificationInfo.Records {
					key, e := url.QueryUnescape(record.S3.Object.Key)
					if e != nil {
						fmt.Print("Error:", err)
						continue
					}

					fmt.Printf("%#v", record)

					dir, _ := path.Split(key)

					b := tx.Bucket("minio/")

					if v, err := b.CreateBucketIfNotExists(dir); err != nil {
						fmt.Print("Error:", err)
						continue
					} else {
						b = v
					}

					var f interface{}
					if err := b.Get(key, &f); err == nil {
					} else if !meta.IsNoSuchObject(err) {
						fmt.Println("Error:", err)
						continue
					} else if i, err := mfs.NextSequence(tx); err != nil {
						fmt.Println("Error:", err)
						continue
					} else {
						oi := record.S3.Object
						f = File{
							Size:  uint64(oi.Size),
							Inode: i,
							UID:   mfs.config.uid,
							GID:   mfs.config.gid,
							Mode:  mfs.config.mode,
							/*
								objectMeta doesn't contain those fields

								Chgtime: oi.LastModified,
								Crtime:  oi.LastModified,
								Mtime:   oi.LastModified,
								Atime:   oi.LastModified,
							*
							Path: "/" + key,
							ETag: oi.ETag,
						}

						if err := f.(*File).store(tx); err != nil {
							fmt.Println("Error:", err)
							continue
						}
					}

				}

				// Commit the transaction and check for error.
				if err := tx.Commit(); err != nil {
					panic(err)
				}

			}
		}()

		return nil
	*/
}

func (mfs *MinFS) stopNotificationListener() error {
	// try to set and listen for notifications
	// Fetch the bucket location.
	location, err := mfs.api.GetBucketLocation(mfs.config.bucket)
	if err != nil {
		return err
	}

	// Fetch any existing bucket notification on the bucket.
	bn, err := mfs.api.GetBucketNotification(mfs.config.bucket)
	if err != nil {
		return err
	}

	accountARN := minio.NewArn("minio", "sns", location, mfs.config.accountID, "listen")

	// Remove account ARN if any.
	bn.RemoveTopicByArn(accountARN)

	// Set back the new sets of notifications.
	return mfs.api.SetBucketNotification(mfs.config.bucket, bn)
}
