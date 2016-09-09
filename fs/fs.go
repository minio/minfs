/*
 * MinFS for Amazon S3 Compatible Cloud Storage (C) 2015, 2016 Minio, Inc.
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

// Package minfs contains the MinFS core package
package minfs

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/minio/minfs/meta"
	"github.com/minio/minfs/queue"
	"github.com/minio/minio-go"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var (
	_ = meta.RegisterExt(1, File{})
	_ = meta.RegisterExt(2, Dir{})
)

// MinFS contains the meta data for the MinFS client
type MinFS struct {
	config *Config
	api    *minio.Client

	db *meta.DB

	// contains all open handles
	handles []*FileHandle

	m sync.Mutex

	queue *queue.Queue

	syncChan chan interface{}
}

// todo(nl5887): update cache with deleted files as well
// todo(nl5887): mkdir

// New will return a new MinFS client
func New(options ...func(*Config)) (*MinFS, error) {
	// set defaults
	cfg := &Config{
		cacheSize: 10000000,
		cache:     "./cache/",
		basePath:  "",
		accountID: fmt.Sprintf("%d", time.Now().UTC().Unix()),
		gid:       0,
		uid:       0,
		mode:      os.FileMode(0660),
	}

	for _, optionFn := range options {
		optionFn(cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	fs := &MinFS{
		config:   cfg,
		syncChan: make(chan interface{}),
	}

	return fs, nil
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
	err = mfs.api.SetBucketNotification(mfs.config.bucket, bn)
	if err != nil {
		return err
	}

	// Success.
	return nil
}

func (mfs *MinFS) updateMetadata() error {
	for {
		// updates metadata periodically. This is being used when notification listener
		// is not available
		time.Sleep(time.Second * 1)
	}
}

func (mfs *MinFS) startNotificationListener() error {
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
						*/
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
}

func (mfs *MinFS) mount() (*fuse.Conn, error) {
	return fuse.Mount(
		mfs.config.mountpoint,
		fuse.FSName("MinFS"),
		fuse.Subtype("MinFS"), // todo: bucket? or amazon /minio?
		fuse.LocalVolume(),
		fuse.VolumeName(mfs.config.bucket), // bucket?
	)
}

// Serve starts the MinFS client
func (mfs *MinFS) Serve() error {
	if mfs.config.debug {
		fuse.Debug = func(msg interface{}) {
			fmt.Printf("%#v\n", msg)
		}
	}

	// initialize
	fmt.Println("Opening cache database...")
	if db, err := meta.Open(path.Join(mfs.config.cache, "cache.db"), 0600, nil); err != nil {
		return err
	} else {
		mfs.db = db
	}

	defer mfs.db.Close()

	fmt.Println("Initializing cache database...")
	if err := mfs.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("minio/"))
		return err
	}); err != nil {
		return err
	}

	fmt.Println("Initializing minio client...")

	host := mfs.config.target.Host
	access := os.Getenv("MINFS_ACCESS")
	secret := os.Getenv("MINFS_SECRET")
	secure := mfs.config.target.Scheme == "https"
	if api, err := minio.New(host, access, secret, secure); err != nil {
		return err
	} else {
		mfs.api = api
	}

	// set notifications
	fmt.Println("Starting notification listener...")
	if err := mfs.startNotificationListener(); err != nil {
		return err
	}

	fmt.Println("Mounting target....")
	// mount the drive
	c, err := mfs.mount()
	if err != nil {
		return err
	}

	defer c.Close()

	if err := mfs.startSync(); err != nil {
		return err
	}

	fmt.Println("Mounted... Have fun!")
	// serve the filesystem
	if err := fs.Serve(c, mfs); err != nil {
		return err
	}

	// todo(nl5887): implement this
	fmt.Println("HOW TO QUIT?")

	// todo(nl5887): move trap signals to Main, this is not supposed to be in Serve
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGUSR1)

loop:
	for {
		// check if the mount process has an error to report
		select {
		case <-c.Ready:
			if err := c.MountError; err != nil {
				log.Fatal(err)
			}
		case s := <-signalCh:
			if s == os.Interrupt {
				return mfs.stopNotificationListener()
			} else if s == syscall.SIGUSR1 {
				fmt.Println("PRINT STATS")
				continue
			}
			break loop
		}
	}

	return nil
}

// Operation -
type Operation struct {
	Error chan error
}

// MoveOperation -
type MoveOperation struct {
	*Operation

	Source string
	Target string
}

// CopyOperation -
type CopyOperation struct {
	*Operation

	Source string
	Target string
}

// PutOperation -
type PutOperation struct {
	*Operation

	Length int64

	Source string
	Target string
}

// NewSizedLimitedReader -
func NewSizedLimitedReader(r io.Reader, length int64) io.Reader {
	return &SizedLimitedReader{
		LimitedReader: &io.LimitedReader{
			R: r,
			N: length,
		},
		length: length,
	}

}

// SizedLimitedReader -
type SizedLimitedReader struct {
	*io.LimitedReader
	length int64
}

// Size - returns the size of the underlying reader.
func (slr *SizedLimitedReader) Size() int64 {
	return slr.length
}

func (mfs *MinFS) sync(req interface{}) error {
	mfs.syncChan <- req
	return nil
}

func (mfs *MinFS) startSync() error {
	go func() {
		for req := range mfs.syncChan {
			// todo(nl5887): do we want retries?

			switch req := req.(type) {
			case *MoveOperation:
				if err := mfs.api.CopyObject(mfs.config.bucket, req.Target, path.Join(mfs.config.bucket, req.Source), minio.NewCopyConditions()); err != nil {
					req.Error <- err
					return
				} else if err := mfs.api.RemoveObject(mfs.config.bucket, req.Source); err != nil {
					req.Error <- err
				} else {
					req.Error <- nil
				}
			case *CopyOperation:
				if err := mfs.api.CopyObject(mfs.config.bucket, req.Target, path.Join(mfs.config.bucket, req.Source), minio.NewCopyConditions()); err != nil {
					req.Error <- err
					return
				} else {
					req.Error <- err
				}
			case *PutOperation:
				r, err := os.Open(req.Source)
				if err != nil {
					req.Error <- err
					return
				}
				defer r.Close()

				// the limited reader will cause truncated files
				// to be uploaded truncated. The file size is the actual file size,
				// the cache file could not be truncated yet
				// the SizedLimitedReader ensures that a Content-Length will be sent,
				// otherwise files with size 0 will not be uploaded
				slr := NewSizedLimitedReader(r, req.Length)

				_, err = mfs.api.PutObject(mfs.config.bucket, req.Target, slr, "application/octet-stream")
				if err != nil {
					fmt.Printf("%#v: %s\n", req, err.Error())

					req.Error <- err
					return
				}

				// todo(nl5887): remove for debugging purposes
				// this is for testing locks
				time.Sleep(time.Second * 2)

				fmt.Printf("Upload finished: %s -> %s.\n", req.Source, req.Target)
				req.Error <- err
			default:
				panic("Unknown type")
			}
		}
	}()

	return nil
}

// Release release the filehandle
func (mfs *MinFS) Release(fh *FileHandle) {
	mfs.m.Lock()
	defer mfs.m.Unlock()

	mfs.handles[fh.handle] = nil
}

// Acquire will return a new FileHandle
func (mfs *MinFS) Acquire(f *File) (*FileHandle, error) {
	mfs.m.Lock()
	defer mfs.m.Unlock()

	/*
		new locking strategy {
		    mfs.locks = map[string]sync.Mutex{}

		    futex?
		    mfs.locks
		}
	*/

	// generate unique cache path
	cachePath := ""
	if v, err := mfs.NewCachePath(); err != nil {
		return nil, err
	} else {
		cachePath = v
	}

	h := &FileHandle{
		f: f,

		cachePath: cachePath,
	}

	mfs.handles = append(mfs.handles, h)

	h.handle = uint64(len(mfs.handles) - 1)
	return h, nil
}

// IsLocked -
func (mfs *MinFS) IsLocked(path string) bool {
	for _, fh := range mfs.handles {
		if fh == nil {
			continue
		}

		if fh.f.Path == path {
			return true
		}
	}

	return false
}

// NextSequence will return the next free iNode
func (mfs *MinFS) NextSequence(tx *meta.Tx) (sequence uint64, err error) {
	bucket := tx.Bucket("minio/")
	return bucket.NextSequence()
}

// Root is the root folder of the MinFS mountpoint
func (mfs *MinFS) Root() (fs.Node, error) {
	return &Dir{
		dir: nil,

		mfs:  mfs,
		Mode: os.ModeDir | 0555,
		Path: "",
	}, nil
}

// Storer -
type Storer interface {
	store(tx *meta.Tx)
}

// NewCachePath -
func (mfs *MinFS) NewCachePath() (string, error) {
	cachePath := path.Join(mfs.config.cache, nextSuffix())
	for {
		if _, err := os.Stat(cachePath); err == nil {
		} else if os.IsNotExist(err) {
			return cachePath, nil
		} else {
			return "", err
		}
		cachePath = path.Join(mfs.config.cache, nextSuffix())
	}
}
