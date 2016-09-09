package minfs

import "io"

// NewSizedLimitedReader -
func NewSizedLimitedReader(r io.Reader, length int64) io.Reader {
	return &SizedLimitedReader{
		LimitedReader: &io.LimitedReader{
			R: r,
			N: length,
		},
		length: length,
	}

}

// SizedLimitedReader -
type SizedLimitedReader struct {
	*io.LimitedReader
	length int64
}

// Size - returns the size of the underlying reader.
func (slr *SizedLimitedReader) Size() int64 {
	return slr.length
}

func (mfs *MinFS) sync(req interface{}) error {
	mfs.syncChan <- req
	return nil
}
