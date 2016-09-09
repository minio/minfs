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
	"fmt"
	"io"
	"os"

	"github.com/minio/minfs/meta"

	"bazil.org/fuse"
	"golang.org/x/net/context"
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
	fmt.Println("fsync", f.FullPath())
	return nil
}

// Release the file handle
func (fh *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	if err := fh.Close(); err != nil {
		return err
	}

	defer fh.f.mfs.Release(fh)

	os.Remove(fh.cachePath)
	return nil
}

// Flush - experimenting with uploading at flush, this slows operations down till it has been
// completely flushed
func (fh *FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	if !fh.dirty {
		return nil
	}

	// todo(nl5887): create function
	sr := PutOperation{
		Source: fh.Name(),
		Target: fh.f.RemotePath(),

		Length: int64(fh.f.Size),

		Operation: &Operation{
			Error: make(chan error),
		},
	}

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
