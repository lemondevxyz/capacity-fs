# capacity-fs
capacity-fs is a fixed-size filesystem abstraction for afero. It does not implement any interfaces; instead it peggy backs off of already existing filesystem implementations.

## usage
First, go get the package.
```shell
go get github.com/lemondevxyz/capacity-fs
```

Then, use it alongside another filesystem and voila:
```go
package main

import (
    "../home/tim/work/capacity-fs"
    "github.com/spf13/afero"
    "os"
)

func main() {
    // Maximum size of 1024 bytes
    // this would return an error if /tmp > 1024
    // start with an empty directory or something similar
    // size = 4096 (base directory size) + 1024 of file content
    maxSize := int64(4096 + 1024)

    os.RemoveAll("/tmp/capacityfs")
    os.Mkdir("/tmp/capacityfs", 0755)
    
    fs, err := capacityfs.NewCapacityFs(afero.NewBasePathFs(afero.NewOsFs(), "/tmp/capacityfs"), maxSize)
    if err != nil {
        panic(err)
    }
    
    // this wouldn't work because directories (by default)
    // have 4096 of disk space.
    err = fs.Mkdir("/dir1", 0755)
    if err != capacityfs.ErrNotEnoughCapacity {
        panic(err)
    }
    
    file, err := fs.OpenFile("/ok.txt", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
    if err != nil {
        panic(err)
    }
    
    // fill the file
    // this shouldn't error out
    _, err = file.Write(make([]byte, 1024))
    if err != nil {
        panic(err)
    }
    
    _, err = file.Write(make([]byte, 1))
    // this will
    if err != capacityfs.ErrNotEnoughCapacity {
        panic(err)
    }
}
```

## gotchas
1. MkdirAll **DOES NOT** use the underlying MkdirAll implementation; it uses **Mkdir**.
2. Create, Mkdir, MkdirAll will all **DELETE** their files/directories if the limit has been exceeded; make sure to stat before Create/Mkdir/MkdirAll

## F.A.Q
### What if I want to change the capacity?
Create a new instance.
### Why does MkdirAll use underlying Mkdir but not MkdirAll?
Well, because that's far more simpler and easier to test than MkdirAll. Plus, isn't MkdirAll a glorified version of `Mkdir`?
### If I remove a file; will the size change?
Yes.
### Is this stable?
Yes, however, I wouldn't recommend using in production. It hasn't been battle-tested yet nor do I declare myself responsible for any unintended file system damages.
