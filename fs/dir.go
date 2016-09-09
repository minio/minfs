package minfs

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/minio/minfs/meta"
	"golang.org/x/net/context"
)

// Dir implements both Node and Handle for the root directory.
type Dir struct {
	mfs *MinFS

	dir *Dir

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

	scanned bool
}

func (dir *Dir) needsScan() bool {
	return !dir.scanned
}

// Attr returns the attributes for the directory
func (dir *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	*a = fuse.Attr{
		Inode:  dir.Inode,
		Size:   dir.Size,
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

// Lookup returns the file node, and scans the current dir if necessary
func (dir *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if err := dir.scan(ctx); err != nil {
		return nil, err
	}

	// we are not statting each object here because of performance reasons
	var o interface{} // meta.Object
	if err := dir.mfs.db.View(func(tx *meta.Tx) error {
		b := dir.bucket(tx)
		return b.Get(name, &o)
	}); err == nil {
	} else if true /*meta.IsNoSuchObject(err) */ {
		// todo(nl5887): nosuchobject returns incorrect error
		return nil, fuse.ENOENT
	} else if err != nil {
		return nil, err
	}

	if file, ok := o.(File); ok {
		file.mfs = dir.mfs
		file.dir = dir
		return &file, nil
	} else if subdir, ok := o.(Dir); ok {
		subdir.mfs = dir.mfs
		subdir.dir = dir
		return &subdir, nil
	}

	return nil, fuse.ENOENT
}

// RemotePath returns the full path including parent paths for current dir on the remote
func (dir *Dir) RemotePath() string {
	return path.Join(dir.mfs.config.basePath, dir.FullPath())
}

// FullPath returns the full path including parent paths for current dir
func (dir *Dir) FullPath() string {
	fullPath := ""

	p := dir
	for {
		if p == nil {
			break
		}

		fullPath = path.Join(p.Path, fullPath)

		p = p.dir
	}

	return fullPath
}

func (dir *Dir) scan(ctx context.Context) error {
	if !dir.needsScan() {
		return nil
	}

	tx, err := dir.mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	b := dir.bucket(tx)

	objects := map[string]interface{}{}

	// we'll compare the current bucket contents against our cache folder, and update the cache
	if err := b.ForEach(func(k string, o interface{}) error {
		if k[len(k)-1] == '/' {
			return nil
		}

		objects[k] = &o
		return nil
	}); err != nil {
		return err
	}

	prefix := dir.RemotePath()
	if prefix != "" {
		prefix = prefix + "/"
	}

	// the channel will abort the ListObjectsV2 request
	doneCh := make(chan struct{})
	defer close(doneCh)

	ch := dir.mfs.api.ListObjectsV2(dir.mfs.config.bucket, prefix, false, doneCh)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case message, ok := <-ch:
			if !ok {
				break loop
			}

			key := message.Key[len(prefix):]
			baseKey := path.Base(key)

			// object still exists
			objects[baseKey] = nil

			if strings.HasSuffix(key, "/") {
				var d Dir
				if err := b.Get(baseKey, &d); err == nil {
					d.dir = dir
					d.mfs = dir.mfs
				} else if !meta.IsNoSuchObject(err) {
					return err
				} else if i, err := dir.mfs.NextSequence(tx); err != nil {
					return err
				} else {
					d = Dir{
						dir: dir,

						Path:  baseKey,
						Inode: i,

						Mode: 0770 | os.ModeDir,
						GID:  dir.mfs.config.gid,
						UID:  dir.mfs.config.uid,

						Chgtime: message.LastModified,
						Crtime:  message.LastModified,
						Mtime:   message.LastModified,
						Atime:   message.LastModified,
					}

				}

				if err := d.store(tx); err != nil {
					return err
				}
			} else {
				var f File
				if err := b.Get(baseKey, &f); err == nil {
					f.dir = dir
					f.mfs = dir.mfs

					f.Size = uint64(message.Size)
					f.ETag = message.ETag

					if message.LastModified.After(f.Chgtime) {
						f.Chgtime = message.LastModified
					}

					if message.LastModified.After(f.Crtime) {
						f.Crtime = message.LastModified
					}

					if message.LastModified.After(f.Mtime) {
						f.Mtime = message.LastModified
					}

					if message.LastModified.After(f.Atime) {
						f.Atime = message.LastModified
					}
				} else if !meta.IsNoSuchObject(err) {
					return err
				} else if i, err := dir.mfs.NextSequence(tx); err != nil {
					return err
				} else {
					f = File{
						dir:  dir,
						Path: baseKey,

						Size:    uint64(message.Size),
						Inode:   i,
						Mode:    dir.mfs.config.mode,
						GID:     dir.mfs.config.gid,
						UID:     dir.mfs.config.uid,
						Chgtime: message.LastModified,
						Crtime:  message.LastModified,
						Mtime:   message.LastModified,
						Atime:   message.LastModified,
						ETag:    message.ETag,
					}

				}

				if err := f.store(tx); err != nil {
					return err
				}
			}
		}
	}

	// cache housekeeping
	for k, o := range objects {
		if o == nil {
			continue
		}

		// purge from cache
		b.Delete(k)

		if _, ok := o.(Dir); !ok {
			continue
		}

		b.DeleteBucket(k + "/")
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	dir.scanned = true
	return nil
}

// ReadDirAll will return all files in current dir
func (dir *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	if err := dir.scan(ctx); err != nil {
		return nil, err
	}

	var entries = []fuse.Dirent{}

	// update cache folder with bucket list
	if err := dir.mfs.db.View(func(tx *meta.Tx) error {
		return dir.bucket(tx).ForEach(func(k string, o interface{}) error {
			if file, ok := o.(File); ok {
				file.dir = dir
				entries = append(entries, file.Dirent())
			} else if subdir, ok := o.(Dir); ok {
				subdir.dir = dir
				entries = append(entries, subdir.Dirent())
			} else {
				panic("Could not find type. Try to remove cache.")
			}

			return nil
		})
	}); err != nil {
		return nil, err
	}

	return entries, nil
}

func (dir *Dir) bucket(tx *meta.Tx) *meta.Bucket {
	// root folder
	fmt.Printf("BUCKET %s %#v\n", dir.Path, *dir)

	if dir.dir == nil {
		return tx.Bucket("minio/")
	}

	b := dir.dir.bucket(tx)

	return b.Bucket(dir.Path + "/")
}

// Mkdir will make a new directory below current dir
func (dir *Dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	subdir := Dir{
		dir: dir,
		mfs: dir.mfs,

		Path: req.Name,

		Mode: 0770 | os.ModeDir,
		GID:  dir.mfs.config.gid,
		UID:  dir.mfs.config.uid,

		Chgtime: time.Now(),
		Crtime:  time.Now(),
		Mtime:   time.Now(),
		Atime:   time.Now(),
	}

	tx, err := dir.mfs.db.Begin(true)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	if err := subdir.store(tx); err != nil {
		return nil, err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &subdir, nil
}

// Remove will delete a file or directory from current directory
func (dir *Dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	if err := dir.mfs.wait(path.Join(dir.FullPath(), req.Name)); err != nil {
		return err
	}

	tx, err := dir.mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	b := dir.bucket(tx)

	var o interface{}
	if err := b.Get(req.Name, &o); err != nil /*meta.IsNoSuchObject(err) */ {
		return fuse.ENOENT
	} else if err != nil {
		return err
	} else if err := b.Delete(req.Name); err != nil {
		return err
	}

	if req.Dir {
		b.DeleteBucket(req.Name + "/")
	}

	if err := dir.mfs.api.RemoveObject(dir.mfs.config.bucket, path.Join(dir.RemotePath(), req.Name)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// store the dir object in cache
func (dir *Dir) store(tx *meta.Tx) error {
	// directories will be stored in their parent buckets
	b := dir.dir.bucket(tx)

	subbucketPath := path.Base(dir.Path)
	if _, err := b.CreateBucketIfNotExists(subbucketPath + "/"); err != nil {
		return err
	}

	return b.Put(subbucketPath, dir)
}

// Dirent will return the fuse Dirent for current dir
func (dir *Dir) Dirent() fuse.Dirent {
	return fuse.Dirent{
		Inode: dir.Inode, Name: dir.Path, Type: fuse.DT_Dir,
	}
}

// Create will return a new empty file in current dir, if the file is currently locked, it will
// wait for the lock to be freed.
func (dir *Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	if err := dir.mfs.wait(path.Join(dir.FullPath(), req.Name)); err != nil {
		return nil, nil, err
	}

	tx, err := dir.mfs.db.Begin(true)
	if err != nil {
		return nil, nil, err
	}

	defer tx.Rollback()

	b := dir.bucket(tx)

	name := req.Name

	var f File
	if err := b.Get(name, &f); err == nil {
		f.mfs = dir.mfs
		f.dir = dir
	} else if i, err := dir.mfs.NextSequence(tx); err != nil {
		return nil, nil, err
	} else {
		f = File{
			mfs: dir.mfs,
			dir: dir,

			Size:    uint64(0),
			Inode:   i,
			Path:    req.Name,
			Mode:    req.Mode, // dir.mfs.config.mode, // should we use same mode for scan?
			UID:     dir.mfs.config.uid,
			GID:     dir.mfs.config.gid,
			Chgtime: time.Now().UTC(),
			Crtime:  time.Now().UTC(),
			Mtime:   time.Now().UTC(),
			Atime:   time.Now().UTC(),
			ETag:    "",

			// req.Umask
		}
	}

	if err := f.store(tx); err != nil {
		return nil, nil, err
	}

	var fh *FileHandle
	if v, err := dir.mfs.Acquire(&f); err != nil {
		return nil, nil, err
	} else {
		fh = v
	}

	fh.dirty = true
	if fh.cachePath, err = dir.mfs.NewCachePath(); err != nil {
		return nil, nil, err
	}

	if f, err := os.OpenFile(fh.cachePath, int(req.Flags), dir.mfs.config.mode); err == nil {
		fh.File = f
	} else {
		return nil, nil, err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	resp.Handle = fuse.HandleID(fh.handle)
	return &f, fh, nil
}

// Rename will rename files
func (dir *Dir) Rename(ctx context.Context, req *fuse.RenameRequest, nd fs.Node) error {
	// todo(nl5887): lock old file
	// todo(nl5887): lock new file
	// todo(nl5887): check (and update) locks

	tx, err := dir.mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	b := dir.bucket(tx)

	newDir := nd.(*Dir)

	var o interface{}
	if err := b.Get(req.OldName, &o); err != nil {
		return err
	} else if file, ok := o.(File); ok {
		file.dir = dir

		if err := b.Delete(file.Path); err != nil {
			return err
		}

		oldPath := file.RemotePath()

		file.Path = req.NewName
		file.dir = newDir
		file.mfs = dir.mfs

		// todo(nl5887): make function
		sr := MoveOperation{
			Source: oldPath,
			Target: file.RemotePath(),
			Operation: &Operation{
				Error: make(chan error),
			},
		}

		if err := dir.mfs.sync(&sr); err == nil {
		} else if true /*meta.IsNoSuchObject(err) */ {
			// todo(nl5887): nosuchobject returns incorrect error
			return fuse.ENOENT
		} else if err != nil {
			return err
		}

		// we'll wait for the request to be uploaded and synced, before
		// releasing the file
		if err := <-sr.Error; err != nil {
			return err
		}

		if err := file.store(tx); err != nil {
			return err
		}

	} else if subdir, ok := o.(Dir); ok {
		// rescan in case of abort / partial / failure
		// this will repair the cache
		dir.scanned = false

		if err := b.Delete(req.OldName); err != nil {
			return err
		}

		if err := b.DeleteBucket(req.OldName + "/"); err != nil {
			return err
		}

		newDir.scanned = false

		// todo(nl5887): fusebug?
		// the cached node is still invalid, contains the old name
		// but there is no way to retrieve the old node to update the new
		// name. refreshing the parent node won't fix the issue when
		// direct access. Fuse should add the targetnode (subdir) as well,
		// that can be updated.

		subdir.Path = req.NewName
		subdir.dir = newDir
		subdir.mfs = dir.mfs

		if err := subdir.store(tx); err != nil {
			return err
		}

		oldPath := path.Join(dir.RemotePath(), req.OldName)

		doneCh := make(chan struct{})
		defer close(doneCh)

		// todo(nl5887): should we queue operations, so it
		// will live restart?
		ch := dir.mfs.api.ListObjectsV2(dir.mfs.config.bucket, oldPath+"/", true, doneCh)
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case message, ok := <-ch:
				if !ok {
					break loop
				}

				newPath := path.Join(newDir.RemotePath(), req.NewName, message.Key[len(oldPath):])

				// todo(nl5887): make function
				sr := MoveOperation{
					Source: message.Key,
					Target: newPath,
					Operation: &Operation{
						Error: make(chan error),
					},
				}

				if err := dir.mfs.sync(&sr); err == nil {
				} else if true /*meta.IsNoSuchObject(err) */ {
					// todo(nl5887): nosuchobject returns incorrect error
					return fuse.ENOENT
				} else if err != nil {
					return err
				}

				// we'll wait for the request to be uploaded and synced, before
				// releasing the file
				if err := <-sr.Error; err != nil {
					return err
				}
			}
		}
	} else {
		return fuse.ENOSYS
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
