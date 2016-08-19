package minfs

import (
	"fmt"
	"os"
	"path"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/minio/minfs/meta"
	"golang.org/x/net/context"
)

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

	UID uint32
	GID uint32

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
		Uid:    dir.UID,
		Gid:    dir.GID,
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

func (dir *Dir) bucket(tx *meta.Tx) *meta.Bucket {
	b := tx.Bucket("minio")
	return b.Bucket(dir.Path)
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

func (dir *Dir) store(tx *meta.Tx) error {
	b := tx.Bucket("minio")

	subbucketPath := path.Dir(dir.Path)
	if b, err := b.CreateBucketIfNotExists(subbucketPath); err != nil {
		return err
	} else {
		return b.Put(path.Base(dir.Path), dir)
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
			UID:     dir.mfs.config.uid,
			GID:     dir.mfs.config.gid,
			Chgtime: time.Now().UTC(),
			Crtime:  time.Now().UTC(),
			Mtime:   time.Now().UTC(),
			Atime:   time.Now().UTC(),
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
