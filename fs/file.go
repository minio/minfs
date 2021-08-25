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

package minfs

import (
	"fmt"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/minio/minfs/meta"
	minio "github.com/minio/minio-go/v7"
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

// Attr - attr file context.
func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	*a = fuse.Attr{
		Inode:  f.Inode,
		Size:   f.Size,
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
	// update cache with new attributes
	return f.mfs.db.Update(func(tx *meta.Tx) error {
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

		return f.store(tx)
	})
}

// RemotePath will return the full path on bucket
func (f *File) RemotePath() string {
	return path.Join(f.dir.RemotePath(), f.Path)
}

// FullPath will return the full path
func (f *File) FullPath() string {
	return path.Join(f.dir.FullPath(), f.Path)
}

// Saves a new file at cached path and fetches the object based on
// the incoming fuse request.
func (f *File) cacheSave(ctx context.Context, path string, req *fuse.OpenRequest) error {

	if _, err := os.Stat(path); err == nil {
		fmt.Println("Already cached!!")
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if req.Flags&fuse.OpenTruncate == fuse.OpenTruncate {
		f.Size = 0
		return nil
	}

	fmt.Println ("Getting Object:", f.RemotePath())
	object, err := f.mfs.api.GetObject(ctx, f.mfs.config.bucket, f.RemotePath(), minio.GetObjectOptions{})
	if err != nil {
		if meta.IsNoSuchObject(err) {
			return fuse.ENOENT
		}
		return err
	}
	defer object.Close()

	hasher := sha256.New()
	size, err := io.Copy(file, io.TeeReader(object, hasher))
	if err != nil {
		return err
	}

	// update actual file size
	f.Size = uint64(size)

	// hash will be used when encrypting files
	_ = hasher.Sum(nil)

	// Success.
	return nil
}

// Saves a new file at cached path and fetches the object based on
// the incoming fuse request.
func (f *File) cacheAllocate(ctx context.Context) (string, error) {

	fmt.Println ("Statting Object:", f.RemotePath())
	object, err := f.mfs.api.StatObject(ctx, f.mfs.config.bucket, f.RemotePath(), minio.GetObjectOptions{})
	if err != nil {
		if meta.IsNoSuchObject(err) {
			return "", fuse.ENOENT
		}
		return "", err
	}
	// Success.
	// NewCachePath -
	cachePath := path.Join(f.mfs.config.cache, object.ETag)

	return cachePath, err
}

// Open return a file handle of the opened file
func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	fmt.Println("Open()")

	if err := f.dir.mfs.wait(f.Path); err != nil {
		return nil, err
	}

	// Start a writable transaction.
	tx, err := f.mfs.db.Begin(true)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	// cachePath, err := f.dir.mfs.NewCachePath()
	// if err != nil {
	// 	return nil, err
	// }
	cachePath, err := f.cacheAllocate(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Println("Caching at:", cachePath)
	// fmt.Println("MD5 at:", md5Path)

	err = f.cacheSave(ctx, cachePath, req)
	if err != nil {
		return nil, err
	}

	fh, err := f.mfs.Acquire(f)
	if err != nil {
		return nil, err
	}

	fh.cachePath = cachePath

	fh.File, err = os.OpenFile(fh.cachePath, int(req.Flags), f.mfs.config.mode)
	if err != nil {
		return nil, err
	}

	if err = f.store(tx); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	resp.Handle = fuse.HandleID(fh.handle)
	return fh, nil
}

func (f *File) bucket(tx *meta.Tx) *meta.Bucket {
	b := f.dir.bucket(tx)
	return b
}

// Getattr returns the file attributes
func (f *File) Getattr(ctx context.Context, req *fuse.GetattrRequest, resp *fuse.GetattrResponse) error {
	resp.Attr = fuse.Attr{
		Inode:  f.Inode,
		Size:   f.Size,
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

// Dirent returns the File object as a fuse.Dirent
func (f *File) Dirent() fuse.Dirent {
	return fuse.Dirent{
		Inode: f.Inode, Name: f.Path, Type: fuse.DT_File,
	}
}

func (f *File) delete(tx *meta.Tx) error {
	// purge from cache
	b := f.bucket(tx)
	return b.Delete(f.Path)
}
