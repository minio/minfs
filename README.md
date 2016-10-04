# MinFS
MinFS is a fuse driver for Amazon S3 compatible object storage server. Use it to store photos, videos, VMs, containers, log files, or any blob of data as objects on your object storage server. This fuse driver is meant to be used for legacy applications with object storage.

[BoltDB](https://github.com/boltdb/bolt) is used for caching and saving metadata, list of files, permissions, owners etc. _NOTE: Be careful it is always possible to remove boltdb cache. Cache will be recreated by MinFS synchronizing metadata from the server.

## Install from RPMs.

Download the [latest RPM](https://github.com/minio/minfs/releases/download/RELEASE.2016-10-03T06-23-33Z/minfs-0.0.20161003062333-1.x86_64.rpm) from here.

```sh
$ sudo yum install minfs-0.0.20161003062333-1.x86_64.rpm -y
```

## Install from source.

Source installation is only intended for developers and advanced users. If you do not have a working Golang environment, please follow [How to install Golang](https://docs.minio.io/docs/how-to-install-golang).


```sh
$ go get -u github.com/minio/minfs
```


## Adding your credentials in `config.json`.

Before mounting you need to update access credentials in `config.json`. By default `config.json` is generated at `/etc/minfs`. `config.json` comes with default empty credentials which needs to be updated with your S3 server credentials. This is a one time activity and the same `config.json` can be copied on all the other `minfs` nodes as well.

```sh
$ vi /etc/minfs/config.json
```

Default `config.json`.
```json
{"version":"1","accessKey":"","secretKey":""}
```

`config.json` updated with access credentials.
```json
{"version":"1","accessKey":"Q3AM3UQ867SPQQA43P2F","secretKey":"zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG"}
```

Once your have successfully updated `config.json`, we will proceed to mounting.

## Mount on Linux

Before mounting you need to know the endpoint of your S3 server and also the `bucketName` that you are going to use.
```
$ sudo mount -t minfs http://172.16.84.1:9000/asiatrip /hello
```

## Mount on Linux `/etc/fstab`

`/etc/fstab` can be edited as well in following format as shown below.

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


