WARNING: this is a work in progress version, and in no means ready for testing or usage.

# MinFS
MinFS is a fuse driver for Minio and S3 storage backends. Currently we can list the files, retrieve files. When modifying files, they will be only modified in the cache folder.

[BoltDB](https://github.com/boltdb/bolt) is being used for caching and saving metadata, like file listings, permissions, owners and such. Each folder will have its own Bolt bucket where the contents of the folders are placed into.

The cache folder is being used for the BoltDB cache database and the files being cached or modified. It will be always possible to remove the cache folder and cache database. Be careful that MinFS has synchronised the data to the storage. The cache folder will be recreated.

Files that are modified will be queued and uploaded to storage. We're using a defensive locking strategy, that is making MinFS slower, but we'll have less chance of corruption and data loss. Applications will wait for the file to be downloaded first, or till the file has been flushed to the Minio compatible storage. All files are being opened exclusively.

## Working

The following features are roughly working at the moment:

* list folders and subfolders
* open and read files
* create new files 
* modify existing files 
* move and rename of files
* copy files
* delete files
* change permissions

## Known issues

* renaming directories will cause an error when directly accessing the newly moved folder

## Build

```
$ go get -d github.com/minio/minfs
$ cd $GOPATH/minio/minfs
$ make
```

## Installation on Linux

```
$ sudo ln -s $GOPATH/bin/minfs /sbin/mount.minfs
```

## Installation on OS X

```
$ sudo ln -s $GOPATH/bin/minfs /sbin/mount_minfs
```

## Mount on Linux and OS X

```
$ sudo MINFS_ACCESS=AKIAIOSFODNN7EXAMPLE MINFS_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY mount -t minfs http://172.16.84.1:9000/asiatrip /hello
```

It is possible to mount a directory in a bucket to a mountpoint. Just append the directory to the source url. E.g

```
$ sudo MINFS_ACCESS=AKIAIOSFODNN7EXAMPLE MINFS_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY mount -t minfs http://172.16.84.1:9000/asiatrip/dir1/ /hello
```

## Unmount

```
$ umount /hello
```

## Options

* **GID**: The default gid to assign for files from storage.
* **UID**: The default gid to assign for files from storage.
* **Cache**: Location for cache folder.
* **Path**: The root path in the bucket
* **Debug**: Enables debug logs

## Read

Every operation the latest version will be retrieved. We don't have a method of verifying if the file
has been changed on the provider, so this is the safest and will work in most cases.

## Write

When a **dirty** file has been closed, it will be uploaded to the bucket, when the file is 
completely uploaded it will be unlocked.

## Locking

The locking mechanism is very defensive, only one operation is allowed at a time. This prevents
issues with synchronization and keeps the fuse driver simple.

## Frequently asked questions

* if you cannot unmount, try seeing what files are open on the mount. `lsof |grep mount`

## Scenarios

* create a file
``` 
echo test > /hello/test
```
* append to a file
```
echo test > /hello/test
```
* make directory
```
mkdir /hello/newdir
```
* remove empty directory 
```
rm -rf /hello/hewdir
```
* copy lot of small files
```
cp -r .git /hello/
```
* read and verify a lot of files
```
diff -r .git /hello/.git/
```
* remove directory with contents
```
rm -rf /hello/.git
```
* rename file
```
mv /hello/test /hello/test2
```
* move file into different directory
```
mv /hello/test2 /hello/newdir/test2
```
* move directory with contents
```
mv /hello/newdir /hello/newdir2
```
* check locked file
```

```

## Todo

There is a long list of todos:

* allow stats to be printed using a signal
* use Minio notifications to actively update metadata 
* one mountpoint per bucket
* each mountpoint will have its own cache folders and can be mounted to one bucket
* use minio configs? .minfs file for keys?
* implement encryption support, (a)symmetric
* mount readonly?
