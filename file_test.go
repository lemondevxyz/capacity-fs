package capacityfs

import (
	"io"
	"os"
	"testing"

	"github.com/matryer/is"
	"github.com/spf13/afero"
)

type badTruncater struct {
	afero.File
}

func (*badTruncater) Truncate(size int64) error {
	return io.ErrUnexpectedEOF
}

func TestFileSize(t *testing.T) {
	is := is.New(t)

	afs := afero.NewMemMapFs()
	is.NoErr(afero.WriteFile(afs, "ok.txt", []byte("asdf"), 0755))

	file, err := afs.OpenFile("ok.txt", os.O_CREATE|os.O_RDWR, 0755)
	is.NoErr(err)

	hasEnoughCapacity := true
	capacity := int64(0)

	fi := &fileSize{
		File:              file,
		hasEnoughCapacity: func(i int64) bool { return hasEnoughCapacity },
		addCapacity: func(i int64) {
			capacity += i
		},
	}

	hasEnoughCapacity = false

	_, err = fi.Write([]byte("new"))
	is.Equal(err, ErrNotEnoughCapacity)

	_, err = fi.WriteAt([]byte("new"), 2)
	is.Equal(err, ErrNotEnoughCapacity)

	hasEnoughCapacity = true
	_, err = fi.Write([]byte("new"))

	is.NoErr(err)
	is.Equal(capacity, int64(3))

	_, err = fi.WriteAt([]byte("new"), 0)
	is.NoErr(err)
	is.Equal(capacity, int64(6))

	fi.File = &badTruncater{file}
	is.Equal(fi.Truncate(6), io.ErrUnexpectedEOF)
	fi.File = file
	err = fi.Truncate(6)
	is.NoErr(err)
	is.Equal(capacity, int64(0))
}
