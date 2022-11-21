package capacityfs

import (
	"io"
	"os"
	"sync"
	"testing"

	"github.com/matryer/is"
	"github.com/spf13/afero"
)

type failStatFs struct {
	afero.Fs
	name string
}

func (f *failStatFs) Stat(name string) (os.FileInfo, error) {
	if f.name == name {
		return nil, io.ErrUnexpectedEOF
	}
	info, err := f.Fs.Stat(name)

	return info, err
}

type badFs struct {
	afero.Fs
}

func (b *badFs) Create(name string) (afero.File, error) {
	return nil, io.ErrUnexpectedEOF
}

func (b *badFs) Open(name string) (afero.File, error) {
	return nil, io.ErrUnexpectedEOF
}

func (b *badFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return nil, io.ErrUnexpectedEOF
}

func (b *badFs) Mkdir(name string, perm os.FileMode) error    { return io.ErrUnexpectedEOF }
func (b *badFs) MkdirAll(name string, perm os.FileMode) error { return io.ErrUnexpectedEOF }
func (b *badFs) Remove(name string) error                     { return io.ErrUnexpectedEOF }
func (b *badFs) RemoveAll(name string) error                  { return io.ErrUnexpectedEOF }

func TestCalculateFsSize(t *testing.T) {
	is := is.New(t)

	afs := afero.NewMemMapFs()

	is.NoErr(afero.WriteFile(afs, "ok.txt", []byte("asdf"), 0755))
	is.NoErr(afero.WriteFile(afs, "ok2.txt", []byte("asdfgh"), 0755))
	is.NoErr(afs.Mkdir("dir", 0755))
	is.NoErr(afero.WriteFile(afs, "dir/last.txt", []byte("oooo"), 0755))

	sum, err := CalculateSizeSum(&failStatFs{afs, "ok.txt"})
	is.Equal(err, io.ErrUnexpectedEOF)

	sum, err = CalculateSizeSum(afs)
	is.NoErr(err)

	is.Equal(int64(4+6+4+42+42), sum)
}

func TestCapacityFsDeferError(t *testing.T) {
	is := is.New(t)
	afs := &capacityFs{Fs: afero.NewMemMapFs()}

	is.NoErr(afero.WriteFile(afs.Fs, "ok.txt", []byte("asdf"), 0755))
	is.True(onlyLast(afs.Stat("ok.txt")) == nil)
	afs.deferError("ok.txt", nil)
	is.True(onlyLast(afs.Stat("ok.txt")) == nil)
	afs.deferError("ok.txt", io.ErrUnexpectedEOF)
	is.True(onlyLastError(afs.Stat("ok.txt")) != nil)
}

// asd
func TestCapacityFsAddCapacity(t *testing.T) {
	is := is.New(t)

	fs := &capacityFs{Fs: afero.NewMemMapFs()}
	fs.addCapacity(5)

	is.Equal(fs.cachedSize, int64(5))
}

func TestCapacityFsHasEnoughCapacity(t *testing.T) {
	is := is.New(t)

	fs := &capacityFs{Fs: afero.NewMemMapFs()}
	is.True(!fs.hasEnoughCapacity(5))
	fs.limitedSize = 5
	is.True(fs.hasEnoughCapacity(5))
}

func TestCapacityStatAndCheckSize(t *testing.T) {
	is := is.New(t)
	fs := &capacityFs{Fs: afero.NewMemMapFs()}

	is.NoErr(fs.statAndCheckSize("404"))

	is.NoErr(afero.WriteFile(fs.Fs, "ok.txt", []byte("asdf"), 0755))
	fs.limitedSize = 3

	is.Equal(fs.statAndCheckSize("ok.txt"), ErrNotEnoughCapacity)
	fs.limitedSize = 6
	is.NoErr(fs.statAndCheckSize("ok.txt"))
	is.Equal(fs.cachedSize, int64(4))
}

func onlyLast(args ...interface{}) interface{} { return args[len(args)-1] }
func onlyLastError(args ...interface{}) error  { return onlyLast(args...).(error) }

func TestCapacityFsBeforeRemove(t *testing.T) {
	is := is.New(t)
	afs := &capacityFs{afero.NewMemMapFs(), 96, 42, sync.RWMutex{}}

	size, err := afs.beforeRemove("404")
	is.Equal(size, int64(0))
	is.NoErr(err)

	is.NoErr(afs.Fs.Mkdir("dir1", 0755))

	is.NoErr(afero.WriteFile(afs, "dir1/ok.txt", []byte("asdf"), 0755))
	size, err = afs.beforeRemove("dir1")
	is.NoErr(err)
	is.Equal(size, int64(42+4))

	afs.Fs = &failStatFs{afs.Fs, "dir1/ok.txt"}
	is.NoErr(afero.WriteFile(afs, "dir1/ok.txt", []byte("asdf"), 0755))
	_, err = afs.beforeRemove("dir1")
	is.Equal(err, io.ErrUnexpectedEOF)

	is.NoErr(afero.WriteFile(afs, "ok.txt", []byte("asdfg"), 0755))
	size, err = afs.beforeRemove("ok.txt")
	is.NoErr(err)
	is.Equal(size, int64(5))
}

type oversizedStatFs struct {
	afero.Fs
	name string
}

type oversizedFileInfo struct {
	os.FileInfo
}

func (f *oversizedFileInfo) Size() int64 { return 99999 }

func (b *oversizedStatFs) Stat(name string) (os.FileInfo, error) {
	stat, err := b.Fs.Stat(name)
	if err != nil {
		return nil, err
	}

	if b.name == name {
		return &oversizedFileInfo{stat}, nil
	}

	return stat, nil
}

func testCreateMkdirMkdirAll(is *is.I, fn func(fs afero.Fs, param string) error) {
	afs := &capacityFs{afero.NewMemMapFs(), 96, 42, sync.RWMutex{}}

	oldFs := afs.Fs
	afs.Fs = &badFs{oldFs}

	err := fn(afs, "err.txt")
	is.Equal(err, io.ErrUnexpectedEOF)

	afs.Fs = &oversizedStatFs{oldFs, "oversized.txt"}

	err = fn(afs, "oversized.txt")
	is.Equal(err, ErrNotEnoughCapacity)

	_, err = afs.Stat("oversized.txt")
	is.True(err != nil)

	afs.Fs = oldFs

	err = fn(afs, "ok.txt")
	is.NoErr(err)
}

func TestCapacityFsCreate(t *testing.T) {
	testCreateMkdirMkdirAll(is.New(t), func(fs afero.Fs, param string) error {
		_, err := fs.Create(param)
		return err
	})
}

func TestCapacityFsMkdir(t *testing.T) {
	testCreateMkdirMkdirAll(is.New(t), func(fs afero.Fs, param string) error {
		return fs.Mkdir(param, 0755)
	})
}

func TestCapacityFsMkdirAll(t *testing.T) {
	testCreateMkdirMkdirAll(is.New(t), func(fs afero.Fs, param string) error {
		return fs.MkdirAll(param, 0755)
	})

	is := is.New(t)

	cfs, err := NewCapacityFs(afero.NewMemMapFs(), 1024)
	is.NoErr(err)

	afs := cfs.(*capacityFs)
	is.NoErr(afs.MkdirAll("dir1/dir2", 0755))
	is.Equal(afs.cachedSize, int64(42*3))

	is.NoErr(afs.MkdirAll("dir1/dir2/dir3/dir4", 0755))
	is.Equal(afs.cachedSize, int64(42*5))
}

func testCapacityOpenOpenFile(is *is.I, fn func(fs afero.Fs, param string) (afero.File, error)) {
	afs := &capacityFs{afero.NewMemMapFs(), 96, 42, sync.RWMutex{}}

	is.True(onlyLast(fn(afs, "404.txt")) != nil)
	is.NoErr(afero.WriteFile(afs, "ok.txt", []byte("asdf"), 0755))

	file, err := fn(afs, "ok.txt")
	is.NoErr(err)
	_, ok := file.(*fileSize)
	is.True(ok)
}

func TestCapacityFsOpen(t *testing.T) {
	testCapacityOpenOpenFile(is.New(t), func(fs afero.Fs, param string) (afero.File, error) {
		return fs.Open(param)
	})
}

func TestCapacityFsOpenFile(t *testing.T) {
	testCapacityOpenOpenFile(is.New(t), func(fs afero.Fs, param string) (afero.File, error) {
		return fs.OpenFile(param, os.O_RDWR, 0755)
	})
}

type removeBadFs struct {
	afero.Fs
}

func (removeBadFs) Remove(str string) error    { return io.ErrUnexpectedEOF }
func (removeBadFs) RemoveAll(str string) error { return io.ErrUnexpectedEOF }

func testCapacityRemoveRemoveAll(is *is.I, fn func(fs afero.Fs, param string) error) {
	// three directories
	capacityfs, err := NewCapacityFs(afero.NewMemMapFs(), 5000)
	is.NoErr(err)

	afs := capacityfs.(*capacityFs)

	oldFs := afs.Fs
	is.NoErr(afs.Mkdir("dir1", 0755))
	is.NoErr(afs.Mkdir("dir1/dir2", 0755))
	is.NoErr(afero.WriteFile(afs, "dir1/dir2/ok.txt", []byte("asdfg"), 0755))

	afs.Fs = &failStatFs{afs.Fs, "dir1/dir2/ok.txt"}
	is.True(fn(afs, "dir1/dir2") != nil)
	is.Equal(fn(afs, "dir1/dir2"), onlyLast(afs.beforeRemove("dir1/dir2")))

	afs.Fs = &removeBadFs{oldFs}
	is.Equal(fn(afs, "dir1/dir2"), io.ErrUnexpectedEOF)

	afs.Fs = oldFs
	if fn(afs, "dir1/dir2") != nil {
		is.NoErr(afs.RemoveAll("dir1/dir2"))
	}

	is.Equal(afs.cachedSize, int64(84))
}

func TestCapacityFsRemove(t *testing.T) {
	testCapacityRemoveRemoveAll(is.New(t), func(fs afero.Fs, param string) error {
		return fs.Remove(param)
	})
}

func TestCapacityFsRemoveAll(t *testing.T) {
	testCapacityRemoveRemoveAll(is.New(t), func(fs afero.Fs, param string) error {
		return fs.RemoveAll(param)
	})
}

type fsName struct {
	afero.Fs
	name string
}

func (f fsName) Name() string { return f.name }

func TestCapacityFsName(t *testing.T) {
	is := is.New(t)
	f := &capacityFs{Fs: fsName{afero.NewMemMapFs(), "fs"}}

	is.Equal(f.Name(), "capacityFs 0 - fs")
}

func TestNewCapacityFs(t *testing.T) {
	is := is.New(t)
	_, err := NewCapacityFs(nil, -1)
	is.True(err != nil)
	_, err = NewCapacityFs(afero.NewMemMapFs(), -1)
	is.True(err != nil)

	afs := afero.NewMemMapFs()
	is.NoErr(afero.WriteFile(afs, "ok.txt", []byte("asdf"), 0755))

	_, err = NewCapacityFs(afs, 0)
	is.True(err != nil)

	oldFs := afs
	afs = &failStatFs{afs, "ok.txt"}
	_, err = NewCapacityFs(afs, 50)
	is.Equal(err, onlyLastError(CalculateSizeSum(afs)))

	afs = oldFs

	_, err = NewCapacityFs(afs, 50)
	is.NoErr(err)
}

func TestPossibleCombinations(t *testing.T) {
	is := is.New(t)

	is.Equal(possibleCombinations([]string{"dir1", "dir2", "dir3"}), []string{"dir1", "dir1/dir2", "dir1/dir2/dir3"})
}
