WARNING: this is a work in progress version, and in no means ready for testing or usage.

# MinFS
MinFS is a fuse driver for Minio and S3 storage backends. Currently we can list the files, retrieve files. When modifying files, they will be only modified in the cache folder.

[BoltDB](https://github.com/boltdb/bolt) is being used for caching and saving metadata, like file listings, permissions, owners and such. Each folder will have its own Bolt bucket where the contents of the folders are placed into.

The cache folder is being used for the BoltDB cache database and the files being cached or modified. It will be always possible to remove the cache folder and cache database. Be careful that MinFS has synchronised the data to the storage. The cache folder will be recreated.

Files that are modified will be queued and uploaded to storage.

## Working

The following features are roughly working at the moment:

* list folders and subfolders
* open and read files
* create new files (not being uploaded to storage yet, only cache)
* modify existing files (not being uploaded to storage yet, only cache)
* delete files
* change permissions

## Build

```
$ go get github.com/minio/minfs
$ go build -o /usr/bin/minfs
```

## Installation

```
$ ln -s /usr/bin/minfs /usr/sbin/mount.minfs
```

## Mount

```
$ mount -t minfs -o gid=0,uid=0,cache=./cache/ http://{access key}:{secret key}@172.16.84.1:9000/asiatrip /hello
```

## Unmount

```
$ umount /hello
```

## Options

* **GID**: The default gid to assign for files from storage.
* **UID**: The default gid to assign for files from storage.
* **Cache**: Location for cache folder.


## Frequently asked questions

* if you cannot unmount, try seeing what files are open on the mount. `lsof |grep mount`


## Todo

There is a long list of todos:

* work on locking
* work on synchronization to Minio
* allow stats to be printed using a signal
* use local cache folder, for most used files. Do we want to register / cache this info in a bolt db?
* cleanup caching folder, have a size limit
* learn what files to cache
* use Minio notifications to actively update metadata and cached files
* one mountpoint per bucket
* use minio notifications if possible, to update cache
* each mountpoint will have its own cache folders and can be mounted to one bucket
* rename files
* use minio configs? .minfs file for keys?
