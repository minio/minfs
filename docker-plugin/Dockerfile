FROM golang:1.7-wheezy

RUN apt-get update && apt-get upgrade -y && apt-get install sudo fuse libfuse2 -y

RUN mkdir -p /usr/share/man/man8

RUN \
    mkdir -p /run/docker/plugins && \
    apt-get install bash git -y && \
    mkdir -p $GOPATH/src/github.com/minio && \
    cd $GOPATH/src/github.com/minio && \
    git clone https://github.com/minio/minfs && \
    cd minfs && \
    make install

COPY minfs-docker-plugin /usr/sbin/minfs-docker-plugin

CMD ["minfs-docker-plugin"]
