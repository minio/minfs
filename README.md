# MinFS Quickstart Guide [![Slack](https://slack.minio.io/slack?type=svg)](https://slack.minio.io) [![Go Report Card](https://goreportcard.com/badge/minio/minfs)](https://goreportcard.com/report/minio/minfs) [![Docker Pulls](https://img.shields.io/docker/pulls/minio/minfs.svg?maxAge=604800)](https://hub.docker.com/r/minio/minfs/)

MinFS is a fuse driver for Amazon S3 compatible object storage server. MinFS lets you mount a remote bucket (from a S3 compatible object store), as if it were a local directory. This allows you to read and write from the remote bucket just by operating on the local mount directory.

MinFS helps legacy applications use modern object stores with minimal config changes. MinFS uses [BoltDB](https://github.com/boltdb/bolt) for caching and saving metadata, list of files, permissions, owners etc.

> Be careful, it is always possible to remove boltdb cache. Cache will be recreated by MinFS synchronizing metadata from the server.

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

> This example uses [play.minio.io:9000](https://play.minio.io:9000)

```json
{"version":"1","accessKey":"Q3AM3UQ867SPQQA43P2F","secretKey":"zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG"}
```

### Mount `mybucket`

Create an `/etc/fstab` entry

```
https://play.minio.io:9000/mybucket /mnt/mounted/mybucket minfs defaults,cache=/tmp/mybucket 0 0
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

## MinFS Docker Volume plugin

MinFS can also be used via the [MinFS Docker volume plugin](https://github.com/minio/minfs/tree/master/docker-plugin). You can mount a local folder onto a Docker container, without having to go through the dependency installation or the mount and unmount operations of MinFS.

### Minimum Requirements

- [Docker Engine](http://docker.com/) v1.13.0 and above.

### Using Docker Compose

Use `docker-compose` to create a volume using the plugin and share the volume with other containers. In the example below the volume is created using the minfs plugin and and used by `nginx` container to serve the static content from the bucket.

```yml
version: '2'
services:
  my-test-server:
    image: nginx
    ports:
      - "80:80"
    volumes:
      - my-test-store:/usr/share/nginx/html:ro

volumes:
  my-test-store:
    driver: minio/minfs
    driver_opts:
      endpoint: https://play.minio.io:9000
      access-key: Q3AM3UQ867SPQQA43P2F
      secret-key: zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG
      bucket: testbucket
      opts: cache=/tmp/my-test-store
```

<blockquote>
Please change the `endpoint`, `access-key`, `secret-key` and `bucket` for your local Minio setup.
</blockquote>

Once you have successfully created `docker-compose.yml` configuration in your current working directory.

```sh
docker-compose up
```

### Using Docker
One can even manually install the plugin, create and the volume using docker.

Install the plugin

```sh
docker plugin install minio/minfs
```

Create a docker volume `my-test-store` using `minio/minfs` driver.

```sh
docker volume create -d minio/minfs \
  --name my-test-store \
  -o endpoint=https://play.minio.io:9000 \
  -o access-key=Q3AM3UQ867SPQQA43P2F \
  -o secret-key=zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG \
  -o bucket=testbucket
  -o opts=cache=/tmp/my-test-store
```

<blockquote>
Please change the `endpoint`, `access-key`, `secret-key`, `bucket` and `opts` for your local Minio setup.
</blockquote>

Once you have successfully created the volume, start a new container with `my-test-store` attached.
In the example below `nginx` container is run to serve pages from the new volume.

```sh
docker run -d --name my-test-server -p 80:80 -v my-test-store:/usr/share/nginx/html:ro nginx
```

### Test `nginx` Service

Either of the above steps create a MinFS based volume for a Nginx container. Verify if your nginx container is running properly and serving content.

```sh
curl localhost
```

```html
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
  body {
   width: 35em;
   margin: 0 auto;
   font-family: Tahoma, Verdana, Arial, sans-serif;
  }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
```
