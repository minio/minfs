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

// Package minfs contains the MinFS core package
package minfs

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/minio/minfs/meta"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

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

	syncChan chan interface{}

	listenerDoneCh chan struct{}
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
		config:         cfg,
		syncChan:       make(chan interface{}),
		locks:          map[string]bool{},
		log:            log.New(logW, "MinFS ", log.Ldate|log.Ltime|log.Lshortfile),
		listenerDoneCh: make(chan struct{}),
	}

	// Success..
	return fs, nil
}

func (mfs *MinFS) mount() (*fuse.Conn, error) {
	return fuse.Mount(
		mfs.config.mountpoint,
		fuse.FSName("MinFS"),
		fuse.Subtype("MinFS"),
		fuse.LocalVolume(),
		fuse.VolumeName(mfs.config.bucket),
		fuse.AllowOther(),
		fuse.DefaultPermissions(),
	)
}

// Serve starts the MinFS client
func (mfs *MinFS) Serve() (err error) {
	if mfs.config.debug {
		fuse.Debug = func(msg interface{}) {
			mfs.log.Printf("%#v\n", msg)
		}
	}

	defer mfs.shutdown()

	mfs.log.Println("Mounting target....")
	// mount the drive
	var c *fuse.Conn
	c, err = mfs.mount()
	if err != nil {
		return err
	}

	defer c.Close()

	// channel to receive errors
	trapCh := signalTrap(os.Interrupt, syscall.SIGTERM, os.Kill)

	go func() {
		<-trapCh

		mfs.shutdown()
	}()

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

	var (
		host   = mfs.config.target.Host
		access = mfs.config.accessKey
		secret = mfs.config.secretKey
		token  = mfs.config.secretToken
		secure = mfs.config.target.Scheme == "https"
	)

	var transport http.RoundTripper = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: mfs.config.insecure,
		},
		// Set this value so that the underlying transport round-tripper
		// doesn't try to auto decode the body of objects with
		// content-encoding set to `gzip`.
		//
		// Refer:
		//    https://golang.org/src/net/http/transport.go?h=roundTrip#L1843
		DisableCompression: true,
	}

	creds := credentials.NewStaticV4(access, secret, token)
	options := &minio.Options{
		Creds:     creds,
		Secure:    secure,
		Transport: transport,
	}

	mfs.api, err = minio.New(host, options)
	if err != nil {
		return err
	}

	// Validate if the bucket is valid and accessible.
	exists, err := mfs.api.BucketExists(context.Background(), mfs.config.bucket)
	if err != nil {
		return err
	}
	if !exists {
		mfs.log.Println("Bucket doesn't not exist... aborting")
		return os.ErrNotExist
	}

	if err = mfs.startSync(); err != nil {
		return err
	}

	mfs.log.Println("Serving... Have fun!")
	// Serve the filesystem
	if err = fs.Serve(c, mfs); err != nil {
		mfs.log.Println("Error while serving the file system.", err)
		return err
	}

	<-c.Ready
	return c.MountError
}

func (mfs *MinFS) shutdown() {
	fuse.Unmount(mfs.config.mountpoint)
	mfs.log.Println("MinFS stopped cleanly.")
}

func (mfs *MinFS) sync(req interface{}) error {
	mfs.syncChan <- req
	return nil
}

func (mfs *MinFS) moveOp(req *MoveOperation) {
	dst := minio.CopyDestOptions{
		Bucket: mfs.config.bucket,
		Object: req.Target,
	}
	src := minio.CopySrcOptions{
		Bucket: mfs.config.bucket,
		Object: req.Source,
	}
	if _, err := mfs.api.CopyObject(context.Background(), dst, src); err != nil {
		req.Error <- err
		return
	}
	req.Error <- mfs.api.RemoveObject(context.Background(), mfs.config.bucket, req.Source, minio.RemoveObjectOptions{})
}

func (mfs *MinFS) copyOp(req *CopyOperation) {
	dst := minio.CopyDestOptions{
		Bucket: mfs.config.bucket,
		Object: req.Target,
	}
	src := minio.CopySrcOptions{
		Bucket: mfs.config.bucket,
		Object: req.Source,
	}
	_, err := mfs.api.CopyObject(context.Background(), dst, src)
	req.Error <- err
}

func (mfs *MinFS) putOp(req *PutOperation) {
	r, err := os.Open(req.Source)
	if err != nil {
		req.Error <- err
		return
	}
	defer r.Close()

	ops := minio.PutObjectOptions{
		ContentType: mime.TypeByExtension(filepath.Ext(req.Target)),
	}
	_, err = mfs.api.PutObject(context.Background(), mfs.config.bucket, req.Target, r, req.Length, ops)
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
		dir:  nil,
		mfs:  mfs,
		Path: "",

		UID:  mfs.config.uid,
		GID:  mfs.config.gid,
		Mode: os.ModeDir | 0750,
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
