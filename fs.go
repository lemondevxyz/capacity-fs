package capacityfs

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/spf13/afero"
)

// CalculateSizeSum is a function that takes in a file system and walks
// through that filesystem to get the total size of the filesystem.
//
// This function will only return an error if a directory has been read
// through ReadDir but the file couldn't be Stat'd.
func CalculateSizeSum(f afero.Fs) (int64, error) {
	var size int64 = 0

	return size, afero.Walk(f, "", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		size += info.Size()

		return nil
	})
}

type capacityFs struct {
	afero.Fs
	limitedSize int64
	cachedSize  int64
	mtx         sync.RWMutex
}

// hasEnoughCapacity checks whether or not size can be added or not.
func (a *capacityFs) hasEnoughCapacity(size int64) bool {
	a.mtx.RLock()
	defer a.mtx.RUnlock()
	return (size + a.cachedSize) <= a.limitedSize
}

// addCapacity adds the size to the structure's cachedSize
func (a *capacityFs) addCapacity(size int64) {
	a.mtx.Lock()
	a.cachedSize += size
	a.mtx.Unlock()
}

// statAndCheckSize returns an error if the size of a file is more than
// the limited size.
//
// This is only called on Create, Mkdir, MkdirAll.
func (a *capacityFs) statAndCheckSize(name string) error {
	stat, err := a.Fs.Stat(name)
	if err == nil {
		size := stat.Size()

		if !a.hasEnoughCapacity(size) {
			return ErrNotEnoughCapacity
		}

		a.addCapacity(size)
	}

	return nil
}

// DEPRECATED
// deferError removes a file
func (a *capacityFs) deferError(name string, err error) {
	if err != nil {
		a.Fs.Remove(name)
	}
}

// Create creates a file but fails if the size of the file is more than
// the limited size.
func (a *capacityFs) Create(name string) (afero.File, error) {
	fi, err := a.Fs.Create(name)
	if err != nil {
		return nil, err
	}

	fi, err = &fileSize{fi, a.hasEnoughCapacity, a.addCapacity}, a.statAndCheckSize(name)
	if err != nil {
		a.Fs.Remove(name)
	}
	return fi, err
}

// Mkdir creates a directory but could fail if the directory size is
// more than the limited size.
func (a *capacityFs) Mkdir(name string, perm os.FileMode) error {
	err := a.Fs.Mkdir(name, perm)
	if err != nil {
		return err
	}

	err = a.statAndCheckSize(name)
	if err != nil {
		a.Fs.Remove(name)
	}
	return err
}

// possibleCombinations returns a slice of possible path combinations.
//
// E.g. ["dir1", "dir2", "dir3"] => ["dir1", "dir1/dir2", "dir1/dir2/dir3"]
func possibleCombinations(str []string) (ret []string) {
	for k := range str {
		ret = append(ret, strings.Join(str[:k+1], "/"))
	}

	return ret
}

// MkdirAll is a wrapper around Mkdir that creates all directories that
// do not exist.
//
// i.e. /dir1/dir2/dir3/dir4
//
// If dir1 exists, but dir2 doesn't it creates it and its subchildren.
func (a *capacityFs) MkdirAll(name string, perm os.FileMode) (err error) {
	for _, v := range possibleCombinations(strings.Split(name, "/")) {
		_, err := a.Fs.Stat(v)
		if err == nil {
			continue
		}

		err = a.Mkdir(v, perm)
		if err != nil {
			return err
		}
	}

	return nil
}

// Open opens a File with a size limit
func (a *capacityFs) Open(name string) (fi afero.File, err error) {
	fi, err = a.Fs.Open(name)
	if err != nil {
		return
	}

	return &fileSize{fi, a.hasEnoughCapacity, a.addCapacity}, nil
}

// OpenFile opens a File with a size limit
func (a *capacityFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	fi, err := a.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	return &fileSize{fi, a.hasEnoughCapacity, a.addCapacity}, nil
}

// beforeRemove essentially returns the size of the file or the real
// size of a directory.
func (a *capacityFs) beforeRemove(name string) (int64, error) {
	var size int64 = 0
	stat, err := a.Fs.Stat(name)
	if err == nil {
		if stat.IsDir() {
			size, err = CalculateSizeSum(afero.NewBasePathFs(a.Fs, name))
			if err != nil {
				return size, err
			}
		} else {
			size = stat.Size()
		}
	}

	return size, nil
}

// Remove a file in a directory and also syncs the current
// filesystem size.
func (a *capacityFs) Remove(name string) error {
	size, err := a.beforeRemove(name)
	if err != nil {
		return err
	}

	err = a.Fs.Remove(name)
	if err != nil {
		return err
	}

	a.addCapacity(size * -1)

	return nil
}

// RemoveAll removes a directory and its subchildren or a file and also
// syncs the current filesystem size.
func (a *capacityFs) RemoveAll(name string) error {
	size, err := a.beforeRemove(name)
	if err != nil {
		return err
	}

	err = a.Fs.RemoveAll(name)
	if err != nil {
		return err
	}

	a.addCapacity(size * -1)

	return nil
}

// Name modifies the name of the filesystem.
func (a *capacityFs) Name() string {
	return fmt.Sprintf("capacityFs %d - %s", a.limitedSize, a.Fs.Name())
}

// NewCapacityFs is a function that returns a filesystem of a specific
// capacity.
//
// This function will return an error if the filesystem was above the
// provided capacity.
func NewCapacityFs(f afero.Fs, size int64) (afero.Fs, error) {
	if f == nil {
		return nil, fmt.Errorf("nil interface")
	} else if size < 0 {
		return nil, fmt.Errorf("size less than 0")
	}

	cachedSize, err := CalculateSizeSum(f)
	if err != nil {
		return nil, err
	}

	if cachedSize > size {
		return nil, fmt.Errorf("%w: %d > %d", ErrNotEnoughCapacity, cachedSize, size)
	}

	return &capacityFs{
		Fs:          f,
		limitedSize: size,
		cachedSize:  cachedSize,
	}, nil
}
