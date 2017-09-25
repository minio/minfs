FROM golang:1.8-stretch

WORKDIR /go/src/github.com/minio

ADD minfs /go/src/github.com/minio/minfs

RUN \
    mkdir -p {/run/docker/plugins,/usr/share/man/man8} && \
    apt-get update && apt-get upgrade -y && \
    apt-get install sudo fuse libfuse2 bash git -y && \
    cd minfs && \
    make install

COPY minfs-docker-plugin /usr/sbin/minfs-docker-plugin

CMD ["minfs-docker-plugin"]
