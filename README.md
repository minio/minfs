# MinFS
MinFS is a fuse driver for Amazon S3 compatible object storage server. Use it to store photos, videos, VMs, containers, log files, or any blob of data as objects on your object storage server. This fuse driver is meant to be used for legacy applications with object storage.

[BoltDB](https://github.com/boltdb/bolt) is used for caching and saving metadata, list of files, permissions, owners etc. _NOTE_: Be careful it is always possible to remove boltdb cache. Cache will be recreated by MinFS synchronizing metadata from the server.

## Install

Source installation is only intended for developers and advanced users. If you do not have a working Golang environment, please follow [How to install Golang](https://docs.minio.io/docs/how-to-install-golang).


```sh
go get -u -d github.com/minio/minfs
cd $GOPATH/src/github.com/minio/minfs
make
make install
```

## Adding your credentials in `config.json`

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
mkdir -p /testbucket
sudo mount -t minfs https://play.minio.io:9000/testbucket /testbucket
```

## Mount on Linux `/etc/fstab`

`/etc/fstab` can be edited as well in following format as shown below.

```
$ grep minfs /etc/fstab
https://play.minio.io:9000/testbucket /testbucket minfs defaults 0 0
```

## Unmount

```
umount /testbucket
```

