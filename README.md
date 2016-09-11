WARNING: this is a work in progress version, and in no means ready for testing or usage.

# MinFS
MinFS is a fuse driver for Amazon S3 compatible object storage server. Use it to store photos, videos, VMs, containers, log files, or any blob of data as objects on your object storage server. This fuse driver is meant to be used for legacy applications with object storage.

[BoltDB](https://github.com/boltdb/bolt) is used for caching and saving metadata, list of files, permissions, owners etc. _NOTE: Be careful it is always possible to remove boltdb cache. Cache will be recreated by MinFS synchronizing metadata from the server._

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

* Renaming directories will cause an error when directly accessing the newly moved folder

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

For every operation the latest version will be retrieved from provider. For now we don't have a method of verifying if the file has been changed on the provider.

## Write

When a **dirty** file has been closed, it will be uploaded to the bucket, when the file is completely uploaded it will be unlocked.

## Locking

The locking mechanism is very defensive, only one operation is allowed at a time. This prevents issues with synchronization and keeps the fuse driver simple.

## Frequently asked questions

* If you cannot unmount, try seeing what files are open on the mount. `lsof |grep mount`

## Scenarios

* Create a file
```
echo test > /hello/test
```
* Append to a file
```
echo test > /hello/test
```
* Make directory
```
mkdir /hello/newdir
```
* Remove empty directory
```
rm -rf /hello/hewdir
```
* Copy lot of small files
```
cp -r .git /hello/
```
* Read and verify a lot of files
```
diff -r .git /hello/.git/
```
* Remove directory with contents
```
rm -rf /hello/.git
```
* Rename file
```
mv /hello/test /hello/test2
```
* Move file into different directory
```
mv /hello/test2 /hello/newdir/test2
```
* Move directory with contents
```
mv /hello/newdir /hello/newdir2
```

## TODO

There is a long list of todos:

* Allow stats to be printed using a signal.
* Use Minio notifications to actively update metadata.
* One mountpoint per bucket.
* Each mountpoint will have its own cache folders and can be mounted to one bucket
* Implement encryption support, (a)symmetric
