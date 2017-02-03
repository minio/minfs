package fuse

import (
	"bytes"
	"fmt"
	"os/exec"
)

func unmount(dir string) error {
	bin, err := fusermountBinary()
	if err != nil {
		return err
	}
	errBuf := bytes.Buffer{}
	cmd := exec.Command(bin, "-u", dir)
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if errBuf.Len() > 0 {
		return fmt.Errorf("%s (code %v)", errBuf.String(), err)
	}
	return err
}
