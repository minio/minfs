PLUGIN_NAME=minio/minfs
PLUGIN_TAG=latest
PLUGIN_ARCH=amd64 # known namespaces: https://github.com/docker-library/official-images#architectures-other-than-amd64

# Default is to build development plugin.
all: clean docker rootfs create-edge

# Release is to build release plugin.
release: clean docker-release rootfs create-release

clean:
	@echo "### rm ./plugin"
	@rm -rf ./plugin minfs-docker-plugin ./minfs

docker:
	@echo "### docker build: builder image"
	@docker build --build-arg ARCH=${PLUGIN_ARCH} -q -t builder -f Dockerfile.rootfs .
	@echo "### extract minfs-docker-plugin"
	@docker create --name tmp builder
	@docker cp tmp:/go/bin/minfs-docker-plugin .
	@docker rm -vf tmp
	@docker rmi builder
	@echo "### Copy minfs source"
	@mkdir -p minfs
	@cp -a ../minfs* minfs
	@cp -a ../mount.minfs minfs
	@cp -a ../fs minfs
	@cp -a ../meta minfs
	@cp -a ../docs minfs
	@cp -a ../cmd minfs
	@cp -a ../buildscripts minfs
	@cp -a ../Makefile minfs
	@cp -a ../.git minfs
	@echo "### docker build: rootfs image with minfs-docker-plugin"
	@docker build --build-arg ARCH=${PLUGIN_ARCH} -q -t ${PLUGIN_NAME}:rootfs .

docker-release:
	@echo "### docker build: builder image"
	@docker build --build-arg ARCH=${PLUGIN_ARCH} -q -t builder -f Dockerfile.rootfs .
	@echo "### extract minfs-docker-plugin"
	@docker create --name tmp builder
	@docker cp tmp:/go/bin/minfs-docker-plugin .
	@docker rm -vf tmp
	@docker rmi builder
	@echo "### docker build: rootfs image with minfs-docker-plugin"
	@docker build --build-arg ARCH=${PLUGIN_ARCH} -f Dockerfile.release -q -t ${PLUGIN_NAME}:rootfs .

rootfs:
	@echo "### create rootfs directory in ./plugin/rootfs"
	@mkdir -p ./plugin/rootfs
	@docker create --name tmp ${PLUGIN_NAME}:rootfs
	@docker export tmp | tar -x -C ./plugin/rootfs
	@echo "### copy config.json to ./plugin/"
	@cp config.json ./plugin/
	@docker rm -vf tmp

create-release:
	@echo "### remove existing plugin ${PLUGIN_NAME}:latest if exists"
	@docker plugin rm -f ${PLUGIN_NAME}:latest || true
	@echo "### create new plugin ${PLUGIN_NAME}:latest from ./plugin"
	@docker plugin create ${PLUGIN_NAME}:latest ./plugin
	@echo "### enable plugin ${PLUGIN_NAME}:latest"
	@docker plugin enable ${PLUGIN_NAME}:latest

create-edge:
	@echo "### remove existing plugin ${PLUGIN_NAME}:edge if exists"
	@docker plugin rm -f ${PLUGIN_NAME}:edge || true
	@echo "### create new plugin ${PLUGIN_NAME}:edge from ./plugin"
	@docker plugin create ${PLUGIN_NAME}:edge ./plugin

push:  clean docker rootfs create
	@echo "### push plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker plugin push ${PLUGIN_NAME}:${PLUGIN_TAG}
