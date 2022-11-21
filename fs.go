package capacityfs

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/spf13/afero"
)

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

func (a *capacityFs) hasEnoughCapacity(size int64) bool {
	a.mtx.RLock()
	defer a.mtx.RUnlock()
	return (size + a.cachedSize) <= a.limitedSize
}

func (a *capacityFs) addCapacity(size int64) {
	a.mtx.Lock()
	a.cachedSize += size
	a.mtx.Unlock()
}

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

func (a *capacityFs) deferError(name string, err error) {
	if err != nil {
		a.Fs.Remove(name)
	}
}

func (a *capacityFs) Create(name string) (afero.File, error) {
	fi, err := a.Fs.Create(name)
	if err != nil {
		return nil, err
	}

	fi, err = &fileSize{fi, a.hasEnoughCapacity, a.addCapacity}, a.statAndCheckSize(name)
	if err != nil {
		a.deferError(name, err)
	}
	return fi, err
}

func (a *capacityFs) Mkdir(name string, perm os.FileMode) error {
	err := a.Fs.Mkdir(name, perm)
	if err != nil {
		return err
	}

	err = a.statAndCheckSize(name)
	if err != nil {
		a.deferError(name, err)
	}
	return err
}

func possibleCombinations(str []string) (ret []string) {
	for k := range str {
		ret = append(ret, strings.Join(str[:k+1], "/"))
	}

	return ret
}

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

func (a *capacityFs) Open(name string) (fi afero.File, err error) {
	fi, err = a.Fs.Open(name)
	if err != nil {
		return
	}

	return &fileSize{fi, a.hasEnoughCapacity, a.addCapacity}, nil
}

func (a *capacityFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	fi, err := a.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	return &fileSize{fi, a.hasEnoughCapacity, a.addCapacity}, nil
}

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

func (a *capacityFs) Name() string {
	return fmt.Sprintf("capacityFs %d - %s", a.limitedSize, a.Fs.Name())
}

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
