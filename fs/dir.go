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

import (
	"os"
	"path"
	"strings"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/minio/minfs/meta"
	minio "github.com/minio/minio-go"
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
	} else if meta.IsNoSuchObject(err) {
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

func (dir *Dir) storeFile(bucket *meta.Bucket, tx *meta.Tx, baseKey string, objInfo minio.ObjectInfo) error {
	var f File
	err := bucket.Get(baseKey, &f)
	if err == nil {
		// Object already exists and accessible, update values as needed.
		f.dir = dir
		f.mfs = dir.mfs
		f.Size = uint64(objInfo.Size)
		f.ETag = objInfo.ETag
		if objInfo.LastModified.After(f.Chgtime) {
			f.Chgtime = objInfo.LastModified
		}
		if objInfo.LastModified.After(f.Crtime) {
			f.Crtime = objInfo.LastModified
		}
		if objInfo.LastModified.After(f.Mtime) {
			f.Mtime = objInfo.LastModified
		}
		if objInfo.LastModified.After(f.Atime) {
			f.Atime = objInfo.LastModified
		}
	} else if meta.IsNoSuchObject(err) {
		// Object not found, allocate a new inode.
		var seq uint64
		seq, err = dir.mfs.NextSequence(tx)
		if err != nil {
			return err
		}
		f = File{
			dir:     dir,
			Path:    baseKey,
			Size:    uint64(objInfo.Size),
			Inode:   seq,
			Mode:    dir.mfs.config.mode,
			GID:     dir.mfs.config.gid,
			UID:     dir.mfs.config.uid,
			Chgtime: objInfo.LastModified,
			Crtime:  objInfo.LastModified,
			Mtime:   objInfo.LastModified,
			Atime:   objInfo.LastModified,
			ETag:    objInfo.ETag,
		}
		if err = f.store(tx); err != nil {
			return err
		}
	} // else {
	// Returns failure for all other errors.
	return err
}

func (dir *Dir) storeDir(bucket *meta.Bucket, tx *meta.Tx, baseKey string, objInfo minio.ObjectInfo) error {
	var d Dir
	err := bucket.Get(baseKey, &d)
	if err == nil {
		// Prefix already exists and accessible, update values as needed.
		d.dir = dir
		d.mfs = dir.mfs
	} else if meta.IsNoSuchObject(err) {
		// Prefix not found allocate a new inode and create a new directory.
		var seq uint64
		seq, err = dir.mfs.NextSequence(tx)
		if err != nil {
			return err
		}
		d = Dir{
			dir:   dir,
			Path:  baseKey,
			Inode: seq,
			Mode:  0770 | os.ModeDir,
			GID:   dir.mfs.config.gid,
			UID:   dir.mfs.config.uid,

			Chgtime: objInfo.LastModified,
			Crtime:  objInfo.LastModified,
			Mtime:   objInfo.LastModified,
			Atime:   objInfo.LastModified,
		}
		if err = d.store(tx); err != nil {
			return err
		}
	} // else {
	// For all other errors this operation fails.
	return err
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
		if k[len(k)-1] != '/' {
			objects[k] = &o
		}
		return nil
	}); err != nil {
		return err
	}

	prefix := dir.RemotePath()
	if prefix != "" {
		prefix = prefix + "/"
	}

	// The channel will abort the ListObjectsV2 request.
	doneCh := make(chan struct{})
	defer close(doneCh)

	ch := dir.mfs.api.ListObjectsV2(dir.mfs.config.bucket, prefix, false, doneCh)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case objInfo, ok := <-ch:
			if !ok {
				break loop
			}
			key := objInfo.Key[len(prefix):]
			baseKey := path.Base(key)

			// object still exists
			objects[baseKey] = nil

			if strings.HasSuffix(key, "/") {
				dir.storeDir(b, tx, baseKey, objInfo)
			} else {
				dir.storeFile(b, tx, baseKey, objInfo)
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
	// Root folder.
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
	if err := b.Get(req.Name, &o); meta.IsNoSuchObject(err) {
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

	return tx.Commit()
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
	if gerr := b.Get(name, &f); gerr == nil {
		f.mfs = dir.mfs
		f.dir = dir
	} else if i, nerr := dir.mfs.NextSequence(tx); nerr != nil {
		return nil, nil, nerr
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

	if serr := f.store(tx); serr != nil {
		return nil, nil, serr
	}

	var fh *FileHandle
	if fh, err = dir.mfs.Acquire(&f); err != nil {
		return nil, nil, err
	}
	fh.dirty = true
	if fh.cachePath, err = dir.mfs.NewCachePath(); err != nil {
		return nil, nil, err
	}
	if fh.File, err = os.OpenFile(fh.cachePath, int(req.Flags), dir.mfs.config.mode); err != nil {
		return nil, nil, err
	}

	// Commit the transaction and check for error.
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}

	resp.Handle = fuse.HandleID(fh.handle)
	return &f, fh, nil
}

// Rename will rename files
func (dir *Dir) Rename(ctx context.Context, req *fuse.RenameRequest, nd fs.Node) error {
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

		sr := newMoveOp(oldPath, file.RemotePath())
		if err := dir.mfs.sync(&sr); err == nil {
		} else if meta.IsNoSuchObject(err) {
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

		// fusebug?
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

				sr := newMoveOp(message.Key, newPath)
				if err := dir.mfs.sync(&sr); err == nil {
				} else if meta.IsNoSuchObject(err) {
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
	return tx.Commit()
}
