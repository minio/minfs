/*
 * MinioFS for Amazon S3 Compatible Cloud Storage (C) 2015, 2016 Minio, Inc.
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
package main

import (
	"flag"
	"fmt"
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
	"github.com/minio/minio-go"
	"github.com/minio/miniofs/meta"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"golang.org/x/net/context"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", os.Args[0])
	flag.PrintDefaults()
}

var (
	_ = meta.RegisterExt(1, File{})
	_ = meta.RegisterExt(2, Dir{})
)

func init() {
}

func main() {
	MinioFS := MinioFS{}
	MinioFS.Main()
}

type MinioFS struct {
}

// arguments:
// -- debug
// -- bucket
// -- target
// -- permissions
// -- uid / gid

func (mfs *MinioFS) Main() {
	// Enable profiling supported modes are [cpu, mem, block].
	/*
		switch os.Getenv("MINIOFS_PROFILER") {
		case "cpu":
			defer profile.Start(profile.CPUProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
		case "mem":
			defer profile.Start(profile.MemProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
		case "block":
			defer profile.Start(profile.BlockProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
		}
	*/

	/*
	   app := registerApp()
	   app.Before = registerBefore
	           if _, e := pb.GetTerminalWidth(); e != nil {
	                   globalQuiet = true
	           }
	           if globalDebug {
	                   return getSystemData()
	           }
	           return make(map[string]string)
	   }

	   app.RunAndExitOnError()
	*/
	// todo(nl5887): overlay fs? everything not on local cache is from remote?

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	mountpoint := flag.Arg(0)

	fuse.Debug = func(msg interface{}) {
		fmt.Printf("%#v\n", msg)
	}

	// todo(nl5887): do a first scan of root first, that allows to check if credentials are ok.
	bucket := "asiatrip"

	db, err := meta.Open("cache.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	api, err := minio.New("172.16.84.1:9000", "AJBCFEV8M5Q8XIQPRITQ", "TBPnPHamh6r7ypXACfO4Nxz59PjE+3SanplAZDzq", false)
	if err != nil {
		log.Fatalln(err)
	}

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("minio"))
		return err
	})

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("MinioFS"),
		fuse.Subtype("hellofs"), // bucket? or amazon /minio?
		fuse.LocalVolume(),
		fuse.VolumeName(bucket), // bucket?
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	fs1 := &FS{
		api:     api,
		bucket:  bucket,
		db:      db,
		handles: []*File{},
	}
	// todo(nl5887); NewFS()

	accountID := "1"
	fmt.Println("1-")

	// set notifications
	func() {
		// try to set and listen for notifications
		// Fetch the bucket location.
		location, e := api.GetBucketLocation(bucket)
		if e != nil {
			panic(e)
		}

		// Fetch any existing bucket notification on the bucket.
		nConfig, e := api.GetBucketNotification(bucket)
		if e != nil {
			panic(e)
		}

		fmt.Println("1")

		accountARN := minio.NewArn("minio", "sns", location, accountID, "listen")
		// If there are no SNS topics configured, configure the first one.
		shouldSetNotification := len(nConfig.TopicConfigs) == 0
		if !shouldSetNotification {
			// We found previously configure SNS topics, validate if current account-id is the same.
			// this will always set shouldSetNotification right?
			for _, topicConfig := range nConfig.TopicConfigs {
				if topicConfig.Topic == accountARN.String() {
					shouldSetNotification = false
					break
				}
			}
			shouldSetNotification = true
		}
		fmt.Println("2")

		if shouldSetNotification {
			topicConfig := minio.NewNotificationConfig(accountARN)
			topicConfig.AddEvents(minio.ObjectCreatedAll, minio.ObjectRemovedAll)
			nConfig.AddTopic(topicConfig)
			e = api.SetBucketNotification(bucket, nConfig)
			if e != nil {
				panic(e)
			}
		}

		doneCh := make(chan struct{})

		fmt.Println("Notification set")

		// todo(nl5887): reconnect on close
		eventsCh := api.ListenBucketNotification(bucket, accountARN, doneCh)
		go func() {
			for notificationInfo := range eventsCh {
				if notificationInfo.Err != nil {
					continue
				}

				// Start a writable transaction.
				tx, err := db.Begin(true)
				if err != nil {
					panic(err)
				}

				defer tx.Rollback()
				// todo(nl5887): defer not called in for each

				for _, record := range notificationInfo.Records {
					key, e := url.QueryUnescape(record.S3.Object.Key)
					if e != nil {
						fmt.Println("Error: %s", err.Error())
						continue
					}

					dir, name := path.Split(key)

					b := tx.Bucket("minio")

					if v, err := b.CreateBucketIfNotExists("/" + dir); err != nil {
						fmt.Println("Error: %s", err.Error())
						continue
					} else {
						b = v
					}

					var f interface{}
					if err := b.Get(key, &f); err == nil {
					} else if !meta.ErrIsNoSuchObject(err) {
						fmt.Println("Error: %s", err.Error())
						continue
					} else if i, err := fs1.NextSequence(tx); err != nil {
						fmt.Println("Error: %s", err.Error())
						continue
					} else {
						oi := record.S3.Object
						f = File{
							Size:  uint64(oi.Size),
							Inode: i,
							Mode:  0444,
							/*
								objectMeta doesn't contain those fields

								Chgtime: oi.LastModified,
								Crtime:  oi.LastModified,
								Mtime:   oi.LastModified,
								Atime:   oi.LastModified,
							*/
							Path: key,
							ETag: oi.ETag,
						}

						if err := b.Put(name, &f); err != nil {
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
	}()

	err = fs.Serve(c, fs1)
	if err != nil {
		log.Fatal(err)
	}

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
			fmt.Println("Remco Got signal:", s)
			if s == syscall.SIGUSR1 {
				fmt.Println("PRINT STATS")
			} else {
				break loop
			}
		}
	}

	go func() {
		// have a channel doing all get operations
	}()

	go func() {
		// have a channel doing all put operations
	}()

	/*
		// Create a done channel.
		doneCh := make(chan struct{})
		defer close(doneCh)

		// Recurively list all objects in 'mytestbucket'
		recursive := true
		for message := range api.ListObjectsV2("asiatrip", "", recursive, doneCh) {
			fmt.Println(message)
		}
	*/

	/*
	   // determined based on the Endpoint value.

	   // s3Client.TraceOn(os.Stderr)

	   // Create a done channel to control 'ListenBucketNotification' go routine.
	   doneCh := make(chan struct{})

	   // Indicate to our routine to exit cleanly upon return.
	   defer close(doneCh)

	   // Account ARN.
	   accountARN := minio.NewArn("minio", "lambda", "us-east-1", "1", "lambda")

	   // Listen for bucket notifications on "mybucket" filtered by account ARN "arn:minio:sqs:us-east-1:1:minio".
	   for notificationInfo := range minioClient.ListenBucketNotification("YOUR-BUCKETNAME", accountARN, doneCh) {
	           if notificationInfo.Err != nil {
	                   log.Fatalln(notificationInfo.Err)
	           }
	           log.Println(notificationInfo)
	   }
	*/
}

// combine FS and MinioFS structs?
// FS implements the hello world file system., and up to date
type FS struct {
	api    *minio.Client
	bucket string
	db     *meta.DB

	// contains all open handles
	handles []*File
	m       sync.Mutex
}

type FileHandle struct {
	*File
}

func (fs1 *FS) NewHandle(f *File) fs.Handle {
	fs1.m.Lock()
	defer fs1.m.Unlock()

	fmt.Println("NewHandle", fs.Handle(len(fs1.handles)-1))

	// todo(nl5887): FileHandle
	fs1.handles = append(fs1.handles, f)
	return &FileHandle{
		f,
	}
}

// NextSequence will return the next free iNode
func (fs *FS) NextSequence(tx *meta.Tx) (sequence uint64, err error) {
	bucket := tx.Bucket("minio")
	return bucket.NextSequence()
}

func (fs *FS) Root() (fs.Node, error) {
	return &Dir{
		fs:   fs,
		Mode: os.ModeDir | 0555,
		Path: "",
	}, nil
}

// Dir implements both Node and Handle for the root directory.
type Dir struct {
	fs *FS

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
	fmt.Println("Lookup", name)
	/*
		file := File{
			fs:   d.fs,
			path: name,
		}
	*/

	// todo(nl5887): could be called when not yet initialized. for example
	// with empty cache and ls'ing subfolder

	var o interface{}
	if err := d.fs.db.View(func(tx *meta.Tx) error {
		b := tx.Bucket("minio")

		subbucketPath := "/" + d.Path
		fmt.Println(subbucketPath)
		b = b.Bucket(subbucketPath)
		return b.Get(name, &o)
	}); err != nil {
		return nil, err
	}

	if file, ok := o.(File); ok {
		file.fs = d.fs
		return &file, nil
	} else if dir, ok := o.(Dir); ok {
		dir.fs = d.fs
		return &dir, nil
	}

	return nil, fuse.ENOENT
}

func (dir *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	doneCh := make(chan struct{})
	// todo(nl5887): close of closed

	defer close(doneCh)

	// cache only doesn't need writable transaction
	// update cache folder with bucket list
	tx, err := dir.fs.db.Begin(true)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	b := tx.Bucket("minio")

	subbucketPath := "/" + dir.Path
	if child, err := b.CreateBucketIfNotExists(subbucketPath); err != nil {
		return nil, err
	} else {
		b = child
	}

	var entries = []fuse.Dirent{}

	// todo(nl5887): use make with count
	/*
		if err := b.ForEach(func(k string, o interface{}) error {
			if file, ok := o.(File); ok {
				entries = append(entries, file.Dirent())
			} else if dir, ok := o.(Dir); ok {
				entries = append(entries, dir.Dirent())
			}

			return nil
		}); err != nil {
			return nil, err
		}

		return entries, nil
	*/

	// we could do this recursive once to update the cache
	ch := dir.fs.api.ListObjectsV2(dir.fs.bucket, dir.Path, false, doneCh)
loop:
	for {
		select {
		case <-ctx.Done():
			close(doneCh)
		case message, ok := <-ch:
			if !ok {
				break loop
			}

			key := message.Key

			if strings.HasSuffix(key, "/") {
				var d Dir
				if err := b.Get(key, &d); err == nil {
				} else if !meta.ErrIsNoSuchObject(err) {
					return nil, err
				} else if i, err := dir.fs.NextSequence(tx); err != nil {
					return nil, err
				} else {
					// todo(nl5887): check if we need to update, and who'll win?
					d = Dir{
						Mode:  os.ModeDir | 0555,
						Path:  key,
						Inode: i,

						Gid: 1000,
						Uid: 1000,

						Chgtime: message.LastModified,
						Crtime:  message.LastModified,
						Mtime:   message.LastModified,
						Atime:   message.LastModified,
					}

					if err := b.Put(path.Base(key), &d); err != nil {
						return nil, err
					}
				}

				entries = append(entries, d.Dirent())
			} else {
				var f File
				if err := b.Get(key, &f); err == nil {
				} else if !meta.ErrIsNoSuchObject(err) {
					return nil, err
				} else if i, err := dir.fs.NextSequence(tx); err != nil {
					return nil, err
				} else {
					// todo(nl5887): check if we need to update, and who'll win?
					f = File{
						Size:    uint64(message.Size),
						Inode:   i,
						Mode:    0440,
						Gid:     1000,
						Uid:     1000,
						Chgtime: message.LastModified,
						Crtime:  message.LastModified,
						Mtime:   message.LastModified,
						Atime:   message.LastModified,
						Path:    key,
						ETag:    message.ETag,
					}

					if err := b.Put(path.Base(key), &f); err != nil {
						return nil, err
					}
				}

				entries = append(entries, f.Dirent())
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return entries, nil

}

// File implements both Node and Handle for the hello file.
type File struct {
	fs *FS

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
	tx, err := f.fs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	b := tx.Bucket("minio")

	subbucketPath := "/" + f.Path
	fmt.Println("Creating subbucket ", subbucketPath)

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

	if err := b.Put(path.Base(f.Path), &f); err != nil {
		return err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (fh *FileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	fmt.Println("WRITE")
	return nil
}

// Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error
func (fh *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	fmt.Println("READ")
	// make sure bytes are available and cache them
	/*
		    	Header    `json:"-"`
			Dir       bool // is this Readdir?
			Handle    HandleID
			Offset    int64
			Size      int
			Flags     ReadFlags
			LockOwner uint64
			FileFlags OpenFlags
	*/
	// resp.DataReadResponse
	_ = req.Offset
	_ = req.Size

	resp.Data = []byte("TEST")
	return nil
}

func (f *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	fmt.Println("RELEASE", req.Handle)
	fmt.Printf("%#v\n", req)
	return nil
}
func (f *FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	fmt.Println("FLUSH")
	return nil
}

func (file *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	fmt.Println("OPEN")
	return file.fs.NewHandle(file), nil
}

func (file *File) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	fmt.Println("REMOVE")

	// copy and delete the old one?
	return fmt.Errorf("Remove is not supported.")
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
	// Start a writable transaction.
	tx, err := dir.fs.db.Begin(true)
	if err != nil {
		return nil, nil, err
	}

	defer tx.Rollback()

	b := tx.Bucket("minio")

	subbucketPath := "/" + dir.Path
	fmt.Println("Creating subbucket ", subbucketPath)

	b, err = b.CreateBucketIfNotExists(subbucketPath)
	if err != nil {
		fmt.Println("Bucket not exists", subbucketPath)
		panic(err)
	}

	f := File{
		Size:  uint64(0),
		Inode: 0,
		Mode:  req.Mode,
		Path:  path.Join(dir.Path, req.Name),
		ETag:  "",
		// req.Umask
	}
	// todo(nl5887): add last update date

	name := req.Name

	if err := b.Get(name); err != nil {
		panic(err)
	} else if i, err := dir.fs.NextSequence(tx); err != nil {
		panic(err)
	} else {
		f.Inode = i
		// todo(nl5887): only save when changed
	}

	if err := b.Put(req.Name, &f); err != nil {
		panic(err)
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		panic(err)
	}

	// todo(nl5887): fs.NewHandle()
	return &f, dir.fs.NewHandle(&f), nil
}
