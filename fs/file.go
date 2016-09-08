package minfs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/minio/minfs/meta"
	"golang.org/x/net/context"
)

// File implements both Node and Handle for the hello file.
type File struct {
	mfs *MinFS

	dir *Dir

	Path string

	Inode uint64

	Mode os.FileMode

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

	Hash []byte
}

func (f *File) store(tx *meta.Tx) error {
	b := f.bucket(tx)
	return b.Put(path.Base(f.Path), f)
}

// Forget - forgets the fd.
func (f *File) Forget() {
	// TODO: should this be implemented? @y4m4
	fmt.Println("Forget")
}

// Attr - attr file context.
func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	*a = fuse.Attr{
		Inode: f.Inode,
		Size:  f.Size,
		/*
		   Blocks    :file.Size / 512,
		   Nlink     : 1,
		   BlockSize : 512,
		*/
		Atime:  f.Atime,
		Mtime:  f.Mtime,
		Ctime:  f.Chgtime,
		Crtime: f.Crtime,
		Mode:   f.Mode,
		Uid:    f.UID,
		Gid:    f.GID,
		Flags:  f.Flags,
	}

	return nil
}

// Setattr - set attribute.
func (f *File) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	tx, err := f.mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	// update cache
	if req.Valid.Mode() {
		f.Mode = req.Mode
	}

	if req.Valid.Uid() {
		f.UID = req.Uid
	}

	if req.Valid.Gid() {
		f.GID = req.Gid
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

	if err := f.store(tx); err != nil {
		return err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// Lookup returns the directory node
func (f *File) Lookup(ctx context.Context, name string) (fs.Node, error) {
	// todo(nl5887): implenent abort
	// todo(nl5887): stat object?
	panic("STAT")
	return nil, nil
}

// Fsync -
func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	return nil
}

// RemotePath -
func (f *File) RemotePath() string {
	return path.Join(f.dir.RemotePath(), f.Path)
}

// FullPath -
func (f *File) FullPath() string {
	return path.Join(f.dir.FullPath(), f.Path)
}

// Open -
func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	if err := f.dir.wait(f.Path); err != nil {
		return nil, err
	}

	// check req.Flags, if open for writing and already open,
	// then deny. Now we're only allowing single open files.
	var fh *FileHandle
	if v, err := f.mfs.Acquire(f); err != nil {
		return nil, err
	} else {
		fh = v
	}

	// Start a writable transaction.
	tx, err := f.mfs.db.Begin(true)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	fmt.Printf("OPEN FLAGS %#v\n", req.Flags.String())

	if req.Flags&fuse.OpenTruncate == fuse.OpenTruncate {
		fmt.Println("TRUNCATE")
		if file, err := os.OpenFile(fh.cachePath, int(req.Flags), f.mfs.config.mode); err != nil {
			return nil, err
		} else {
			fh.File = file
			f.Size = 0
		}
	} else {
		// todo(nl5887): cleanup
		object, err := f.mfs.api.GetObject(f.mfs.config.bucket, f.RemotePath())
		if err != nil /* todo(nl5887): No such object*/ {
			return nil, fuse.ENOENT
		} else if err != nil {
			return nil, err
		}
		defer object.Close()

		hasher := sha256.New()

		var r io.Reader = object
		r = io.TeeReader(r, hasher)

		file, err := os.Create(fh.cachePath)
		if err != nil {
			return nil, err
		}

		defer file.Close()

		if size, err := io.Copy(file, r); err != nil {
			return nil, err
		} else {
			// update file size
			f.Size = uint64(size)
		}

		// todo(nl5887): do we want to save as hashes? this will deduplicate files in cache file
		// and also introduces some kind of versioning, hasher can be saved in filehandle
		// we only don't have the hashes being returned at the time from the storage
		fmt.Printf("Sum: %#v\n", hasher.Sum(nil))

		f.Hash = hasher.Sum(nil)

		if file, err := os.OpenFile(fh.cachePath, int(req.Flags), f.mfs.config.mode); err != nil {
			return nil, err
		} else {
			fh.File = file
		}
	}

	if err := f.store(tx); err != nil {
		return nil, err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	resp.Handle = fuse.HandleID(fh.handle)
	return fh, nil
}

func (f *File) bucket(tx *meta.Tx) *meta.Bucket {
	b := f.dir.bucket(tx)
	return b
}

// Getattr -
func (f *File) Getattr(ctx context.Context, req *fuse.GetattrRequest, resp *fuse.GetattrResponse) error {
	resp.Attr = fuse.Attr{
		Inode: f.Inode,
		Size:  f.Size,
		/*
		   Blocks    :file.Size / 512,
		   Nlink     : 1,
		   BlockSize : 512,
		*/
		Atime:  f.Atime,
		Mtime:  f.Mtime,
		Ctime:  f.Chgtime,
		Crtime: f.Crtime,
		Mode:   f.Mode,
		Uid:    f.UID,
		Gid:    f.GID,
		Flags:  f.Flags,
	}

	return nil
}

// Dirent -
func (f *File) Dirent() fuse.Dirent {
	return fuse.Dirent{
		Inode: f.Inode, Name: path.Base(f.Path), Type: fuse.DT_File,
	}
}
