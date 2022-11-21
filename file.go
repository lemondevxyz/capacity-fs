package capacityfs

import (
	"fmt"

	"github.com/spf13/afero"
)

type fileSize struct {
	afero.File
	// hasEnoughCapacity is the function that fileSize uses to check
	// if a file can write or not.
	hasEnoughCapacity func(i int64) bool
	// addCapacity is a function that adds the size of the file's content.
	addCapacity func(i int64)
}

var (
	ErrNotEnoughCapacity = fmt.Errorf("not enough capacity")
)

// Write first checks the size of the byte of slices and returns
// an error if it larger than the allowed size. Otherwise, it just
// delegates the Write operation to the underlying file.
func (f *fileSize) Write(p []byte) (n int, err error) {
	if !f.hasEnoughCapacity(int64(len(p))) {
		return 0, ErrNotEnoughCapacity
	}

	n, err = f.File.Write(p)
	f.addCapacity(int64(n))

	return n, err
}

// WriteAt is the same as Write but with an offset.
func (f *fileSize) WriteAt(p []byte, off int64) (n int, err error) {
	if !f.hasEnoughCapacity(int64(len(p))) {
		return 0, ErrNotEnoughCapacity
	}

	n, err = f.File.WriteAt(p, off)
	f.addCapacity(int64(n))

	return n, err
}

// Truncate some of the file's content.
func (f *fileSize) Truncate(size int64) error {
	err := f.File.Truncate(size)
	if err != nil {
		return err
	}

	f.addCapacity(size * -1)
	return err
}
