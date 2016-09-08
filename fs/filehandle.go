package minfs

import (
	"fmt"
	"io"
	"os"

	"bazil.org/fuse"
	"golang.org/x/net/context"
)

type FileHandle struct {
	*os.File

	// names are confusing
	f *File

	// cache file has been written to
	dirty bool

	cachePath string

	handle uint64
}

func (fh *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	buff := make([]byte, req.Size)
	n, err := fh.File.ReadAt(buff, req.Offset)
	if err == io.EOF {
	} else if err != nil {
		return err
	}

	resp.Data = buff[:n]
	return nil
}

func (fh *FileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
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

		fh.dirty = true

		return nil
	}
}

func (fh *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	if err := fh.Close(); err != nil {
		return err
	}

	defer fh.f.mfs.Release(fh)

	os.Remove(fh.cachePath)
	return nil
}

// experimenting with uploading at flush, this slows operations down till it has been
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
	// Start a writable transaction.
	tx, err := fh.f.mfs.db.Begin(true)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	fmt.Printf("%#v\n", *fh)
	fmt.Printf("%#v\n", *fh.f)
	if err := fh.f.store(tx); err != nil {
		return err
	}

	// Commit the transaction and check for error.
	if err := tx.Commit(); err != nil {
		return err
	}

	fh.dirty = false

	return nil
}
