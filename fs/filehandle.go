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
	"context"
	"io"
	"os"

	"bazil.org/fuse"

	"github.com/minio/minfs/meta"
)

// FileHandle - Contains an opened file which can be read from and written to
type FileHandle struct {
	// the os file handle
	*os.File

	// the fuse file
	f *File

	// cache file has been written to
	dirty bool

	cachePath string

	handle uint64
}

// Read from the file handle
func (fh *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	buff := make([]byte, req.Size)
	n, err := fh.File.ReadAt(buff, req.Offset)
	if err != nil && err != io.EOF {
		return err
	}
	resp.Data = buff[:n]
	return nil
}

// Write to the file handle
func (fh *FileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	if _, err := fh.File.Seek(req.Offset, 0); err != nil {
		return err
	}
	n, err := fh.File.Write(req.Data)
	if err != nil {
		return err
	}
	// Writes that grow the file are expected to update the file size
	// (as seen through Attr). Note that file size changes are
	// communicated also through Setattr.
	if fh.f.Size < uint64(req.Offset)+uint64(n) {
		fh.f.Size = uint64(req.Offset) + uint64(n)
	}
	resp.Size = n
	fh.dirty = true
	return nil
}

// Fsync because of bug in fuse lib, this is on file. -- FIXME - needs more context (y4m4).
func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	// fmt.Println("fsync", f.FullPath())
	return nil
}

// Release the file handle
func (fh *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	if err := fh.Close(); err != nil {
		return err
	}

	defer fh.f.mfs.Release(fh)

	// TODO: We were removing the cached file... we can be smarter about cache management...
	// os.Remove(fh.cachePath)
	return nil
}

// Flush - experimenting with uploading at flush, this slows operations down till it has been
// completely flushed
func (fh *FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	if !fh.dirty {
		return nil
	}

	sr := newPutOp(fh.Name(), fh.f.RemotePath(), int64(fh.f.Size))
	if err := fh.f.mfs.sync(&sr); err != nil {
		return err
	}

	// we'll wait for the request to be uploaded and synced, before
	// releasing the file
	if err := <-sr.Error; err != nil {
		return err
	}

	// update cache
	if err := fh.f.mfs.db.Update(func(tx *meta.Tx) error {
		return fh.f.store(tx)
	}); err != nil {
		return err
	}

	fh.dirty = false
	return nil
}
