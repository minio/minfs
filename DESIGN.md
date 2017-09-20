Introduction [![Slack](https://slack.minio.io/slack?type=svg)](https://slack.minio.io)
------------

This fuse driver allows Minio bucket or any bucket on S3 compatible storage to be mounted as a local, as a prerequesite you need [fusermount](http://man7.org/linux/man-pages/man1/fusermount3.1.html). This feature allows Minio to serve a bucket over a minimal POSIX API.

Limitations
----------

### Read

For every operation the latest version will be retrieved from the server. For now we don't have a method of verifying if the file has been changed by the provider.

### Write

When a **dirty** file has been closed, it will be uploaded to the bucket, when the file is completely uploaded it will be unlocked.

### Locking

The locking mechanism is defensive and doesn't implement granular byte range locking from POSIX API, only one operation is allowed at a time per object. This trade-off is intention and kept to keep the fuse driver simpler.

FUSE options
----------

### Options

* **gid**: The default gid to assign for files from storage.
* **uid**: The default gid to assign for files from storage.
* **cache**: Location for cache folder.
* **debug**: Enables debug logs

### Work in Progress.

- Use Minio notifications to actively update metadata.
- One mountpoint per bucket.
- Each mountpoint will have its own cache folders and can be mounted to one bucket.
- Renaming directories will cause an error when directly accessing the newly moved folder.
