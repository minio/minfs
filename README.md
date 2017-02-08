# MinFS Quickstart Guide [![Slack](https://slack.minio.io/slack?type=svg)](https://slack.minio.io) [![Go Report Card](https://goreportcard.com/badge/minio/minfs)](https://goreportcard.com/report/minio/minfs) [![Docker Pulls](https://img.shields.io/docker/pulls/minio/minfs.svg?maxAge=604800)](https://hub.docker.com/r/minio/minfs/)

MinFS is a fuse driver for Amazon S3 compatible object storage server. Use it to store photos, videos, VMs, containers, log files, or any blob of data as objects on your object storage server. This fuse driver is meant to be used for legacy applications with object storage.

[BoltDB](https://github.com/boltdb/bolt) is used for caching and saving metadata, list of files, permissions, owners etc. _NOTE_: Be careful it is always possible to remove boltdb cache. Cache will be recreated by MinFS synchronizing metadata from the server.

## Docker Plugin

```sh
docker plugin install minio/minfs
```

### Create a docker volume using the plugin

```sh
docker volume create -d minio/minfs \
  --name my-test-store \
  -o endpoint=https://play.minio.io:9000 \
  -o access-key=Q3AM3UQ867SPQQA43P2F \
  -o secret-key=zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG \
  -o bucket=testbucket
```

NOTE: Please change the `endpoint`, `access-key` and `secret-key` for your local Minio setup.

### Attach the volume to a new container

```sh
docker run -it -v my-test-store:/data busybox /bin/sh
ls /data
```

## Source

Source installation is only intended for developers and advanced users. If you do not have a working Golang environment, please follow [How to install Golang](https://docs.minio.io/docs/how-to-install-golang).

```sh
go get -u -d github.com/minio/minfs
cd $GOPATH/src/github.com/minio/minfs
make
make install
```

### Add your credentials in `config.json`

Before mounting you need to update access credentials in `config.json`. By default `config.json` is generated at `/etc/minfs`. `config.json` comes with default empty credentials which needs to be updated with your S3 server credentials. This is a one time activity and the same `config.json` can be copied on all the other **MinFS** deployments as well.

```sh
mkdir -p /etc/minfs
vi /etc/minfs/config.json
```

Default `config.json`.

```json
{"version":"1","accessKey":"","secretKey":""}
```

`config.json` updated with your access credentials.

```json
{"version":"1","accessKey":"Q3AM3UQ867SPQQA43P2F","secretKey":"zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG"}
```

Once your have successfully updated `config.json`, we will proceed to mount.

### Mount

Before mounting you need to know the endpoint of your S3 server and also the `bucketName` that you are going to mount.

```sh
mkdir -p /testbucket
mount -t minfs https://play.minio.io:9000/testbucket /testbucket
```
