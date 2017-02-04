FROM golang:1.7-wheezy

WORKDIR /go/src/app

COPY . /go/src/app

RUN \
       apt-get update && \
       apt-get upgrade -y && \
       apt-get install fuse sudo -y && \
       go-wrapper download && \
       make install && \
       mkdir -p /minfs && \
       rm -rf /go/pkg /go/src

ENTRYPOINT ["minfs", "-o", "rw"]
VOLUME ["/minfs"]
