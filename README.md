# MinFS
MinFS is a fuse driver for Amazon S3 compatible object storage server. Use it to store photos, videos, VMs, containers, log files, or any blob of data as objects on your object storage server. This fuse driver is meant to be used for legacy applications with object storage.

[BoltDB](https://github.com/boltdb/bolt) is used for caching and saving metadata, list of files, permissions, owners etc. _NOTE: Be careful it is always possible to remove boltdb cache. Cache will be recreated by MinFS synchronizing metadata from the server.

## Installation

```
$ go get -d github.com/minio/minfs
$ cd $GOPATH/src/github.com/minio/minfs
$ make
```

## Installation on Linux

```
$ sudo cp -pv $GOPATH/bin/minfs /sbin/minfs
$ sudo cp -pv mount.minfs /sbin/mount.minfs
```

## Mount on Linux

```
$ sudo mount -t minfs http://172.16.84.1:9000/asiatrip /hello
```

## Mount on Linux `/etc/fstab`

```
$ grep minfs /etc/fstab
http://172.16.84.1:9000/asiatrip /hello minfs defaults 0 0
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

The locking mechanism is very defensive, only one operation is allowed at a time per object. This prevents issues with synchronization and keeps the fuse driver simple.

## Work in Progress.

- Allow stats to be printed using a signal.
- Use Minio notifications to actively update metadata.
- One mountpoint per bucket.
- Each mountpoint will have its own cache folders and can be mounted to one bucket.
- Renaming directories will cause an error when directly accessing the newly moved folder.  


