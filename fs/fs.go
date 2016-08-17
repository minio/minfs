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
package minfs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/minio/minfs/meta"
	"github.com/minio/minfs/queue"
	"github.com/minio/minio-go"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"golang.org/x/net/context"
)

var (
	_ = meta.RegisterExt(1, File{})
	_ = meta.RegisterExt(2, Dir{})
)

// MinFS
type MinFS struct {
	config *Config
	api    *minio.Client

	db *meta.DB

	// contains all open handles
	handles []*FileHandle

	m sync.Mutex

	queue *queue.Queue
}

func New(options ...func(*Config)) (*MinFS, error) {
	// set defaults
	cfg := &Config{
		cacheSize: 10000000,
		cache:     "./cache/",
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
		config: cfg,
	}
	return fs, nil
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
			return

			for _, record := range notificationInfo.Records {
				key, e := url.QueryUnescape(record.S3.Object.Key)
				if e != nil {
					fmt.Println("Error: %s", err.Error())
					continue
				}

				fmt.Printf("%#v", record)

				dir, _ := path.Split(key)

				b := tx.Bucket("minio")

				if v, err := b.CreateBucketIfNotExists(dir); err != nil {
					fmt.Println("Error: %s", err.Error())
					continue
				} else {
					b = v
				}

				var f interface{}
				if err := b.Get(key, &f); err == nil {
				} else if !meta.IsNoSuchObject(err) {
					fmt.Println("Error: %s", err.Error())
					continue
				} else if i, err := mfs.NextSequence(tx); err != nil {
					fmt.Println("Error: %s", err.Error())
					continue
				} else {
					oi := record.S3.Object
					f = File{
						Size:  uint64(oi.Size),
						Inode: i,
						Uid:   mfs.config.uid,
						Gid:   mfs.config.gid,
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
						fmt.Println("Error: %s", err.Error())
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
	mfs.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("minio"))
		return err
	})

	fmt.Println("Initializing minio client...")

	host := mfs.config.target.Host
	access := mfs.config.target.User.Username()
	secret, _ := mfs.config.target.User.Password()
	secure := (mfs.config.target.Scheme == "https")
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

	// we are doing an initial scan of the filesystem
	fmt.Println("Scanning source bucket....")
	if err := mfs.scan("/"); err != nil {
		return err
	}

	go func() {
		// have a channel doing all get operations
	}()

	go func() {
		// have a channel doing all put operations
	}()

	fmt.Println("Mounting target....")
	// mount the drive
	c, err := mfs.mount()
	if err != nil {
		return err
	}

	defer c.Close()

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
			if s == syscall.SIGUSR1 {
				fmt.Println("PRINT STATS")
				continue
			}

			break loop
		}
	}

	return nil
}

type FileHandle struct {
	*os.File
	// names are confusing
	f *File

	handle uint64
}

func (mfs *MinFS) NewHandle(f *File) *FileHandle {
	mfs.m.Lock()
	defer mfs.m.Unlock()

	h := &FileHandle{
		f: f,
	}

	mfs.handles = append(mfs.handles, h)

	h.handle = uint64(len(mfs.handles))
	return h
}

// NextSequence will return the next free iNode
func (mfs *MinFS) NextSequence(tx *meta.Tx) (sequence uint64, err error) {
	bucket := tx.Bucket("minio")
	return bucket.NextSequence()
}

// Root is the root folder of the MinFS mountpoint
func (mfs *MinFS) Root() (fs.Node, error) {
	return &Dir{
		mfs:  mfs,
		Mode: os.ModeDir | 0555,
		Path: "/",
	}, nil
}

// Dir implements both Node and Handle for the root directory.
type Dir struct {
	mfs *MinFS

	Path  string
	Inode uint64
	Mode  os.FileMode

	Size uint64
	ETag string

	Atime time.Time
	Mtime time.Time

	Uid uint32
	Gid uint32

	// OS X only
	Bkuptime time.Time
	Chgtime  time.Time
	Crtime   time.Time
	Flags    uint32 // see chflags(2)
}

func (dir *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	*a = fuse.Attr{
		Inode: dir.Inode,
		Size:  dir.Size,
		/*
		   Blocks    :dir.Size / 512,
		   Nlink     : 1,
		   BlockSize : 512,
		*/
		Atime:  dir.Atime,
		Mtime:  dir.Mtime,
		Ctime:  dir.Chgtime,
		Crtime: dir.Crtime,
		Mode:   dir.Mode,
		Uid:    dir.Uid,
		Gid:    dir.Gid,
		Flags:  dir.Flags,
	}

	return nil
}

// todo(nl5887): implement cancel
// todo(nl5887): implement removed files
// todo(nl5887): buckets in buckets in buckets? or just subbuckets in minio bucket?

func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	// todo(nl5887): could be called when not yet initialized. for example
	// with empty cache and ls'ing subfolder

	var o interface{} // meta.Object
	if err := d.mfs.db.View(func(tx *meta.Tx) error {
		b := d.bucket(tx)
		return b.Get(name, &o)
	}); err == nil {
	} else if true /* todo(nl5887): check for no such object */ {
		return nil, fuse.ENOENT
	} else if err != nil {
		return nil, err
	}

	if file, ok := o.(File); ok {
		file.mfs = d.mfs
		return &file, nil
	} else if dir, ok := o.(Dir); ok {
		dir.mfs = d.mfs
		return &dir, nil
	}

	return nil, fuse.ENOENT
}

func (mfs *MinFS) scan(p string) error {
	tx, err := mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	b := tx.Bucket("minio")

	if child, err := b.CreateBucketIfNotExists(p); err != nil {
		return err
	} else {
		b = child
	}

	doneCh := make(chan struct{})
	defer close(doneCh)

	ch := mfs.api.ListObjectsV2(mfs.config.bucket, p[1:], false, doneCh)

	for message := range ch {
		key := message.Key

		if strings.HasSuffix(key, "/") {
			var d Dir
			if err := b.Get(key, &d); err == nil {
			} else if !meta.IsNoSuchObject(err) {
				return err
			} else if i, err := mfs.NextSequence(tx); err != nil {
				return err
			} else {
				// todo(nl5887): check if we need to update, and who'll win?
				d = Dir{
					Path:  "/" + key,
					Inode: i,

					Mode: 0770 | os.ModeDir,
					Gid:  mfs.config.gid,
					Uid:  mfs.config.uid,

					Chgtime: message.LastModified,
					Crtime:  message.LastModified,
					Mtime:   message.LastModified,
					Atime:   message.LastModified,
				}

				if err := b.Put(path.Base(key), &d); err != nil {
					return err
				}
			}
		} else {
			var f File
			if err := b.Get(key, &f); err == nil {
			} else if !meta.IsNoSuchObject(err) {
				return err
			} else if i, err := mfs.NextSequence(tx); err != nil {
				return err
			} else {
				// todo(nl5887): check if we need to update, and who'll win?
				f = File{
					Size:    uint64(message.Size),
					Inode:   i,
					Mode:    mfs.config.mode,
					Gid:     mfs.config.gid,
					Uid:     mfs.config.uid,
					Chgtime: message.LastModified,
					Crtime:  message.LastModified,
					Mtime:   message.LastModified,
					Atime:   message.LastModified,
					Path:    "/" + key,
					ETag:    message.ETag,
				}

				if err := f.store(tx); err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit()
}

func (dir *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	// if not exists then scan
	dir.mfs.scan("/" + dir.Path)

	// cache only doesn't need writable transaction
	// update cache folder with bucket list
	tx, err := dir.mfs.db.Begin(false)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	b := tx.Bucket("minio").Bucket(dir.Path)

	var entries = []fuse.Dirent{}

	// todo(nl5887): use make([]fuse.Dirent{}, count)
	if err := b.ForEach(func(k string, o interface{}) error {
		if file, ok := o.(File); ok {
			entries = append(entries, file.Dirent())
		} else if dir, ok := o.(Dir); ok {
			entries = append(entries, dir.Dirent())
		} else {
			panic("Could not find type. Try to remove cache.")
		}

		return nil
	}); err != nil {
		return nil, err
	}

	fmt.Printf("%#v", entries)

	return entries, nil
}

// File implements both Node and Handle for the hello file.
type File struct {
	mfs *MinFS

	Path string

	Inode uint64

	Mode os.FileMode

	Size uint64
	ETag string

	Atime time.Time
	Mtime time.Time

	Uid uint32
	Gid uint32

	// OS X only
	Bkuptime time.Time
	Chgtime  time.Time
	Crtime   time.Time
	Flags    uint32 // see chflags(2)

	Hash []byte
}

type object interface {
	store(tx *meta.Tx)
}

func (dir *Dir) store(tx *meta.Tx) error {
	b := tx.Bucket("minio")

	subbucketPath := path.Dir(dir.Path)
	if b, err := b.CreateBucketIfNotExists(subbucketPath); err != nil {
		return err
	} else {
		return b.Put(path.Base(dir.Path), dir)
	}
}

func (file *File) store(tx *meta.Tx) error {
	b := tx.Bucket("minio")

	subbucketPath := path.Dir(file.Path)
	if b, err := b.CreateBucketIfNotExists(subbucketPath); err != nil {
		return err
	} else {
		return b.Put(path.Base(file.Path), file)
	}
}

func (file *File) Forget() {
	fmt.Println("Forget")
}

func (file *File) Attr(ctx context.Context, a *fuse.Attr) error {
	*a = fuse.Attr{
		Inode: file.Inode,
		Size:  file.Size,
		/*
		   Blocks    :file.Size / 512,
		   Nlink     : 1,
		   BlockSize : 512,
		*/
		Atime:  file.Atime,
		Mtime:  file.Mtime,
		Ctime:  file.Chgtime,
		Crtime: file.Crtime,
		Mode:   file.Mode,
		Uid:    file.Uid,
		Gid:    file.Gid,
		Flags:  file.Flags,
	}

	return nil
}

// todo(nl5887): take care
/*
// Methods returning Node should take care to return the same Node
// when the result is logically the same instance. Without this, each
// Node will get a new NodeID, causing spurious cache invalidations,
// extra lookups and aliasing anomalies. This may not matter for a
// simple, read-only filesystem.
*/

/*
func (f *File) ReadAll(ctx context.Context) ([]byte, error) {
	fmt.Println("ReadAll", f.fs.bucket, f.Path)
	// check if cached, and up to date
	object, err := f.fs.api.GetObject(f.fs.bucket, f.Path)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(object)
}
*/

func (f *File) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	tx, err := f.mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	b := tx.Bucket("minio")

	subbucketPath := path.Dir(f.Path)
	b, err = b.CreateBucketIfNotExists(subbucketPath)
	if err != nil {
		fmt.Println("Bucket not exists", subbucketPath)
		return err
	}

	// update cache
	if req.Valid.Mode() {
		f.Mode = req.Mode
	}

	if req.Valid.Uid() {
		f.Uid = req.Uid
	}

	if req.Valid.Gid() {
		f.Gid = req.Gid
	}

	if req.Valid.Size() {
		f.Size = req.Size
		fmt.Println("UPDATED SIZE", req.Size)
	}

	if req.Valid.Atime() {
		f.Atime = req.Atime
	}

	if req.Valid.Mtime() {
		f.Mtime = req.Mtime
	}

	if req.Valid.Handle() {
		// todo(nl5887): what is this?
		// f.Handle = req.Handle
	}

	/*
			if req.Valid&fuse.SetattrAtimeNow == fuse.SetattrAtimeNow {
				f.AtimeNow = req.AtimeNow
			}

			if req.Valid&fuse.SetattrMtimeNow == fuse.SetattrMtimeNow {
				f.MtimeNow = req.MtimeNow
			}

		if req.Valid&fuse.SetattrLockOwner == fuse.SetattrLockOwner {
			f.LockOwner = req.LockOwner
		}
	*/

	if req.Valid.Crtime() {
		f.Crtime = req.Crtime
	}

	if req.Valid.Chgtime() {
		f.Chgtime = req.Chgtime
	}

	if req.Valid.Bkuptime() {
		f.Bkuptime = req.Bkuptime
	}

	if req.Valid.Flags() {
		f.Flags = req.Flags
	}

	if err := f.store(tx); err != nil {
		return err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("Setattr %#v\n", resp.Attr)
	//pretty.Print(f)

	return nil
}

func (fh *FileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	fmt.Println("WRITE", string(req.Data))
	if _, err := fh.File.Seek(req.Offset, 0); err != nil {
		fmt.Println("ERROR", err.Error())
		return err
	}

	if n, err := fh.File.Write(req.Data); err != nil {
		fmt.Println("ERROR", err.Error())
		return err
	} else {
		// Writes that grow the file are expected to update the file size
		// (as seen through Attr). Note that file size changes are
		// communicated also through Setattr.
		if fh.f.Size < uint64(req.Offset)+uint64(n) {
			fh.f.Size = uint64(req.Offset) + uint64(n)
		}

		resp.Size = n
		return nil
	}
}

// Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error
func (fh *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	// check if file is in cache

	buff := make([]byte, req.Size)
	n, err := fh.File.ReadAt(buff, req.Offset)
	if err == io.EOF {
	} else if err != nil {
		return err
	}

	resp.Data = buff[:n]
	return nil
}

func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	return nil
}

func (f *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	err := f.File.Close()
	return err
}

func (f *FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	return f.File.Sync()
}

func (file *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	cachePath := path.Join(file.mfs.config.cache, path.Base(file.Path))

	if _, err := os.Stat(cachePath); err == nil {
	} else if os.IsNotExist(err) {
		object, err := file.mfs.api.GetObject(file.mfs.config.bucket, file.Path[1:])

		if err != nil /* No such object*/ {
			return nil, fuse.ENOENT
		} else if err != nil {
			return nil, err
		}
		defer object.Close()

		// Start a writable transaction.
		tx, err := file.mfs.db.Begin(true)
		if err != nil {
			return nil, err
		}

		defer tx.Rollback()

		hasher := sha256.New()

		var r io.Reader = object
		r = io.TeeReader(r, hasher)

		// todo(nl5887): do we want to have original filenames? or hashes of the filename
		f, err := os.Create(cachePath)
		if err != nil {
			return nil, err
		}

		defer f.Close()

		if _, err := io.Copy(f, r); err != nil {
			return nil, err
		}

		// todo(nl5887): do we want to save as hashes? this will deduplicate files in cache file
		// and also introduces some kind of versioning, hasher can be saved in filehandle
		fmt.Printf("Sum: %#v\n", hasher.Sum(nil))

		file.Hash = hasher.Sum(nil)

		if err := file.store(tx); err != nil {
			return nil, err
		}

		// Commit the transaction and check for error.
		if err := tx.Commit(); err != nil {
			return nil, err
		}

	} else {
		return nil, err
	}

	// update cache bucket!

	fh := file.mfs.NewHandle(file)

	if f, err := os.OpenFile(cachePath, int(req.Flags), file.mfs.config.mode); err != nil {
		return nil, err
	} else {
		fh.File = f
	}

	resp.Handle = fuse.HandleID(fh.handle)
	return fh, nil
}

func (dir *Dir) bucket(tx *meta.Tx) *meta.Bucket {
	b := tx.Bucket("minio")
	return b.Bucket(dir.Path)
}

func (file *File) bucket(tx *meta.Tx) *meta.Bucket {
	b := tx.Bucket("minio")
	return b.Bucket(path.Base(file.Path))
}

func (dir *Dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	tx, err := dir.mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	// stat object to see if it still exists on remote?

	b := dir.bucket(tx)

	var o interface{}
	if err := b.Get(req.Name, &o); err != nil {
		return err
	}

	if err := b.Delete(req.Name); err == nil {
	} else if meta.IsNoSuchObject(err) {
		// what error do we need to return if the object doesn't exist anymore?
		return nil
	} else if err != nil {
		return err
	}

	if req.Dir {
		b.DeleteBucket(req.Name)
	}

	// check error, if not exists
	if err := dir.mfs.api.RemoveObject(dir.mfs.config.bucket, path.Join(dir.Path, req.Name)); err != nil {
		// what error do we need to return if the object doesn't exist anymore?
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// do we want to delete immediately?
	// or update cache file and add to queue
	// two queues, a read queue and a write / delete queue
	// todo(nl5887) rm -rf?
	return nil
}

func (file *File) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	fmt.Println("RENAME")

	// copy and delete the old one?
	return fmt.Errorf("Rename is not supported.")
}

func (file *File) Getattr(ctx context.Context, req *fuse.GetattrRequest, resp *fuse.GetattrResponse) error {
	resp.Attr = fuse.Attr{
		Inode: file.Inode,
		Size:  file.Size,
		/*
		   Blocks    :file.Size / 512,
		   Nlink     : 1,
		   BlockSize : 512,
		*/
		Atime:  file.Atime,
		Mtime:  file.Mtime,
		Ctime:  file.Chgtime,
		Crtime: file.Crtime,
		Mode:   file.Mode,
		Uid:    file.Uid,
		Gid:    file.Gid,
		Flags:  file.Flags,
	}

	fmt.Printf("Getattr %#v\n", resp.Attr)

	return nil
}

func (file *File) Dirent() fuse.Dirent {
	return fuse.Dirent{
		Inode: file.Inode, Name: path.Base(file.Path), Type: fuse.DT_File,
	}
}

func (dir *Dir) Dirent() fuse.Dirent {
	return fuse.Dirent{
		Inode: dir.Inode, Name: path.Base(dir.Path), Type: fuse.DT_Dir,
	}
}
func (dir *Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	fmt.Printf("CREATE %s\n", req.Name)

	// Start a writable transaction.
	tx, err := dir.mfs.db.Begin(true)
	if err != nil {
		return nil, nil, err
	}

	defer tx.Rollback()

	b := dir.bucket(tx)

	name := req.Name

	cachePath := path.Join(dir.mfs.config.cache, name)

	var f File
	// todo(nl5887): add last update date

	if err := b.Get(name, &f); err == nil {
	} else if i, err := dir.mfs.NextSequence(tx); err != nil {
		return nil, nil, err
	} else {
		f = File{
			Size:    uint64(0),
			Inode:   i,
			Path:    path.Join(dir.Path, req.Name),
			Mode:    req.Mode, // dir.mfs.config.mode, // should we use same mode for scan?
			Gid:     dir.mfs.config.gid,
			Uid:     dir.mfs.config.uid,
			Chgtime: time.Now(),
			Crtime:  time.Now(),
			Mtime:   time.Now(),
			Atime:   time.Now(),
			ETag:    "",

			// req.Umask
			mfs: dir.mfs,
		}
	}

	if err := f.store(tx); err != nil {
		return nil, nil, err
	}

	fh := dir.mfs.NewHandle(&f)

	if f, err := os.Create(cachePath); err == nil {
		fh.File = f
	} else if err != nil {
		return nil, nil, err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	resp.Handle = fuse.HandleID(fh.handle)

	// todo(nl5887): fs.NewHandle() f.NewHandle() ?
	return &f, fh, nil
}
