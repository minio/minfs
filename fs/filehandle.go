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

	handle uint64
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

func (f *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	err := f.File.Close()
	return err
}

func (f *FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	return f.File.Sync()
}
