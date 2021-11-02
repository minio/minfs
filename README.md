# MinFS Quickstart Guide [![Slack](https://slack.min.io/slack?type=svg)](https://slack.min.io) [![Go Report Card](https://goreportcard.com/badge/minio/minfs)](https://goreportcard.com/report/minio/minfs)

> NOTE:	This project is frozen and is not accepting any new features, feel free to send a [pull request](https://docs.github.com/en/github/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/about-pull-requests) for any such features.

MinFS is a fuse driver for Amazon S3 compatible object storage server. MinFS lets you mount a remote bucket (from a S3 compatible object store), as if it were a local directory. This allows you to read and write from the remote bucket just by operating on the local mount directory.

MinFS helps legacy applications use modern object stores with minimal config changes. MinFS uses [BoltDB](https://github.com/boltdb/bolt) for caching and saving metadata, list of files, permissions, owners etc.

> Be careful, it is always possible to remove boltdb cache. Cache will be recreated by MinFS synchronizing metadata from the server.

# Architecture
![architecture](https://raw.githubusercontent.com/minio/minfs/master/MinFS.svg?sanitize=true)

## POSIX Compatibility
> MinFS is not a POSIX conformant filesystem and it does not intend to be one. MinFS is built for legacy applications that needs to access an object store but does not expect strict POSIX compatibility. Please use MinFS if this fits your needs.

Use cases not suitable for MinFS use are:
- Running a database on MinFS such as postgres, mysql etc.
- Running virtual machines on MinFS such as qemu/kvm.
- Running rich POSIX applications which rely on POSIX locks, Extended attribute operations etc.

Some use cases suitable for MinFS are:
- Serving a static web-content with NGINX, Apache2 web servers.
- Serving as backup destination for legacy tools unable to speak S3 protocol.

## MinFS RPMs
### Minimum Requirements
- [RPM Package Manager](http://rpm.org/)

### Install
Download the pre-built RPMs from [here](https://github.com/minio/minfs/releases/tag/RELEASE.2017-02-26T20-20-56Z)
```sh
yum install minfs-0.0.20170226202056-1.x86_64.rpm
```

### Update `config.json`
Create a new `config.json` in /etc/minfs directory with your S3 server access and secret keys.

> This example uses [play.min.io](https://play.min.io)

```json
{"version":"1","accessKey":"Q3AM3UQ867SPQQA43P2F","secretKey":"zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG"}
```

### Mount `mybucket`
Create an `/etc/fstab` entry
```
https://play.min.io/mybucket /mnt/mounted/mybucket minfs defaults,cache=/tmp/mybucket 0 0
```

Now proceed to mount `fstab` entry.
```sh
mount /mnt/mounted/mybucket
```

Verify if `mybucket` is mounted and is accessible.
```
ls -F /mnt/mounted/mybucket
etc/  issue
```
