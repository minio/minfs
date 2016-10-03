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

// Package minfs contains the MinFS core package
package minfs

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"

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

	// Logger instance.
	log *log.Logger

	// contains all open handles
	handles []*FileHandle

	locks map[string]bool

	m sync.Mutex

	queue *queue.Queue

	syncChan chan interface{}
}

// New will return a new MinFS client
func New(options ...func(*Config)) (*MinFS, error) {
	// Initialize config.
	ac, err := InitMinFSConfig()
	if err != nil {
		return nil, err
	}

	// Initialize log file.
	logW, err := os.OpenFile(globalLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	// Set defaults
	cfg := &Config{
		cache:     globalDBDir,
		basePath:  "",
		accountID: fmt.Sprintf("%d", time.Now().UTC().Unix()),
		gid:       0,
		uid:       0,
		accessKey: ac.AccessKey,
		secretKey: ac.SecretKey,
		mode:      os.FileMode(0660),
	}

	for _, optionFn := range options {
		optionFn(cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Initialize MinFS.
	fs := &MinFS{
		config:   cfg,
		syncChan: make(chan interface{}),
		locks:    map[string]bool{},
		log:      log.New(logW, "MinFS ", log.Ldate|log.Ltime|log.Lshortfile),
	}

	// Success..
	return fs, nil
}

func (mfs *MinFS) updateMetadata() error {
	for {
		// Updates metadata periodically. This is being used when notification listener is not available
		time.Sleep(time.Second * 2) // Every 2 secs.
	}
}

func (mfs *MinFS) mount() (*fuse.Conn, error) {
	return fuse.Mount(
		mfs.config.mountpoint,
		fuse.FSName("MinFS"),
		fuse.Subtype("MinFS"),
		fuse.LocalVolume(),
		fuse.VolumeName(mfs.config.bucket),
	)
}

// Serve starts the MinFS client
func (mfs *MinFS) Serve() (err error) {
	if mfs.config.debug {
		fuse.Debug = func(msg interface{}) {
			mfs.log.Printf("%#v\n", msg)
		}
	}

	// Initialize database.
	mfs.log.Println("Opening cache database...")
	mfs.db, err = meta.Open(path.Join(mfs.config.cache, "cache.db"), 0600, nil)
	if err != nil {
		return err
	}
	defer mfs.db.Close()

	mfs.log.Println("Initializing cache database...")
	if err = mfs.db.Update(func(tx *meta.Tx) error {
		_, berr := tx.CreateBucketIfNotExists([]byte("minio/"))
		return berr
	}); err != nil {
		return err
	}

	mfs.log.Println("Initializing minio client...")
	host := mfs.config.target.Host
	access := mfs.config.accessKey
	secret := mfs.config.secretKey
	secure := mfs.config.target.Scheme == "https"
	mfs.api, err = minio.NewV4(host, access, secret, secure)
	if err != nil {
		return err
	}
	// Validate if the bucket is valid and accessible.
	exists, err := mfs.api.BucketExists(mfs.config.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return minio.ErrorResponse{
			BucketName: mfs.config.bucket,
			Code:       "NoSuchBucket",
			Message:    "The specified bucket does not exist",
		}
	}
	// set notifications
	// fmt.Println("Starting notification listener...")
	// if err = mfs.startNotificationListener(); err != nil {
	//	return err
	// }

	mfs.log.Println("Mounting target....")
	// mount the drive
	var c *fuse.Conn
	c, err = mfs.mount()
	if err != nil {
		return err
	}

	defer c.Close()

	if err = mfs.startSync(); err != nil {
		return err
	}

	var wg sync.WaitGroup

	// channel to receive errors
	errorCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		mfs.log.Println("Mounted... Have fun!")
		// Serve the filesystem
		errorCh <- fs.Serve(c, mfs)
	}()

	<-c.Ready
	if err = c.MountError; err != nil {
		return err
	}

	// todo(nl5887): move trap signals to Main, this is not supposed to be in Serve
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, os.Kill, syscall.SIGUSR1)

loop:
	for {
		select {
		case serr := <-errorCh:
			return serr
		case s := <-signalCh:
			if s == os.Interrupt {
				fuse.Unmount(mfs.config.mountpoint)
				break loop
			} else if s == syscall.SIGUSR1 {
				// TODO - implement this.
			}
		}
	}

	wg.Wait()

	// mfs.stopNotificationListener()
	mfs.log.Println("MinFS stopped cleanly.")

	return nil
}

func (mfs *MinFS) sync(req interface{}) error {
	mfs.syncChan <- req
	return nil
}

func (mfs *MinFS) moveOp(req *MoveOperation) {
	// TODO: using copyObject limits the size of the server side copy to 5GB (S3 Spec) limit.
	err := mfs.api.CopyObject(mfs.config.bucket, req.Target, path.Join(mfs.config.bucket, req.Source), minio.NewCopyConditions())
	if err == nil {
		err = mfs.api.RemoveObject(mfs.config.bucket, req.Source)
		if err == nil {
			req.Error <- nil
			return
		}
	}
	req.Error <- err
}

func (mfs *MinFS) copyOp(req *CopyOperation) {
	// TODO: using copyObject limits the size of the server side copy to 5GB (S3 Spec) limit.
	err := mfs.api.CopyObject(mfs.config.bucket, req.Target, path.Join(mfs.config.bucket, req.Source), minio.NewCopyConditions())
	if err == nil {
		req.Error <- nil
		return
	}
	req.Error <- err
}

func (mfs *MinFS) putOp(req *PutOperation) {
	r, err := os.Open(req.Source)
	if err != nil {
		req.Error <- err
		return
	}
	defer r.Close()

	// The limited reader will cause truncated files to be uploaded truncated.
	// The file size is the actual file size, the cache file could not be
	// truncated yet the SizedLimitedReader ensures that a Content-Length
	// will be sent, otherwise files with size 0 will not be uploaded
	slr := NewSizedLimitedReader(r, req.Length)
	_, err = mfs.api.PutObject(mfs.config.bucket, req.Target, slr, "application/octet-stream")
	if err != nil {
		req.Error <- err
		return
	}
	mfs.log.Printf("Upload finished: %s -> %s.\n", req.Source, req.Target)
	req.Error <- nil
}

func (mfs *MinFS) startSync() error {
	go func() {
		for req := range mfs.syncChan {
			switch req := req.(type) {
			case *MoveOperation:
				mfs.moveOp(req)
			case *CopyOperation:
				mfs.copyOp(req)
			case *PutOperation:
				mfs.putOp(req)
			default:
				panic("Unknown type")
			}
		}
	}()
	return nil
}

// Statfs will return meta information on the minio filesystem
func (mfs *MinFS) Statfs(ctx context.Context, req *fuse.StatfsRequest, resp *fuse.StatfsResponse) error {
	resp.Blocks = 0x1000000000
	resp.Bfree = 0x1000000000
	resp.Bavail = 0x1000000000
	resp.Namelen = 32768
	resp.Bsize = 1024
	return nil
}

// Acquire will return a new FileHandle
func (mfs *MinFS) Acquire(f *File) (*FileHandle, error) {
	if err := mfs.Lock(f.FullPath()); err != nil {
		return nil, err
	}

	h := &FileHandle{
		f: f,
	}

	mfs.handles = append(mfs.handles, h)

	h.handle = uint64(len(mfs.handles) - 1)
	return h, nil
}

// Release release the filehandle
func (mfs *MinFS) Release(fh *FileHandle) error {
	if err := mfs.Unlock(fh.f.FullPath()); err != nil {
		return err
	}

	mfs.handles[fh.handle] = nil
	return nil
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
