package capacityfs

import (
	"fmt"

	"github.com/spf13/afero"
)

type fileSize struct {
	afero.File
	hasEnoughCapacity func(i int64) bool
	addCapacity       func(i int64)
}

var (
	ErrNotEnoughCapacity = fmt.Errorf("not enough capacity")
)

func (f *fileSize) Write(p []byte) (n int, err error) {
	if !f.hasEnoughCapacity(int64(len(p))) {
		return 0, ErrNotEnoughCapacity
	}

	n, err = f.File.Write(p)
	f.addCapacity(int64(n))

	return n, err
}

func (f *fileSize) WriteAt(p []byte, off int64) (n int, err error) {
	if !f.hasEnoughCapacity(int64(len(p))) {
		return 0, ErrNotEnoughCapacity
	}

	n, err = f.File.WriteAt(p, off)
	f.addCapacity(int64(n))

	return n, err
}

func (f *fileSize) Truncate(size int64) error {
	err := f.File.Truncate(size)
	if err != nil {
		return err
	}

	f.addCapacity(size * -1)
	return err
}
