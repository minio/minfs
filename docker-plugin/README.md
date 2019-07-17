# A Docker volume plugin for MinFS

## Minimum Requirements

- Docker [1.13.x](http://docker.com/)

## Installation

```sh
docker plugin install minio/minfs
```

## Docker (Simple)

In following `docker-compose` example volume is created and used by another `nginx` container to serve the static content from the bucket. 

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
      # opts: "uid=1000,gid=1000" for custom user/group id
```

<blockquote>
Please change the `endpoint`, `access-key`, `secret-key` and `bucket` for your local MinIO setup.
</blockquote>

Once you have successfully created `docker-compose.yml` configuration in your current working directory.

```sh
docker-compose up
```

## Docker (Advanced)

Using `docker` cli is a multi step process it is recommended that all users try `docker-compose` approach first to avoid any mistakes.

Create a docker volume `my-test-store` using `minio/minfs` driver.

```sh
docker volume create -d minio/minfs \
  --name my-test-store \
  -o endpoint=https://play.minio.io:9000 \
  -o access-key=Q3AM3UQ867SPQQA43P2F \
  -o secret-key=zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG \
  -o bucket=testbucket
```

<blockquote>
Please change the `endpoint`, `access-key`, `secret-key` and `bucket` for your local MinIO setup.
</blockquote>

Once you have successfully created the volume, start a new container with `my-test-store` attached.

```sh
docker run -d --name my-test-server -p 80:80 -v my-test-store:/usr/share/nginx/html:ro nginx
```

## Test `nginx` Service

Verify if your nginx container is running properly and serving content.

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
