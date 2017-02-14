/*
* Minio Cloud Storage, (C) 2017 Minio, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
	"github.com/minio/minio-go"
)

// Used for Plugin discovery.
// Docker identifies the existence of an active plugin process by searching for
// a unit socket file (.sock) in /run/docker/plugins/.
// A unix server is started at the `socketAdress` to enable discovery of this plugin by docker.
const (
	socketAddress = "/run/docker/plugins/minfs.sock"

	defaultLocation = "us-east-1"
)

// `serverconfig` struct is used to store configuration values of the remote Minio server.
// Minfs uses this info to the mount the remote bucket.
// The server info (endpoint, accessKey and secret Key) is passed during creating a docker volume.
// Here is how to do it,
// $ docker volume create -d minfs \
//    --name medical-imaging-store \
//     -o endpoint=https://play.minio.io:9000 -o access-key=Q3AM3UQ867SPQQA43P2F\
//     -o secret-key=zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG -o bucket=test-bucket
type serverConfig struct {
	// Endpoint of the remote Minio server.
	endpoint string
	// `minfs` mounts the remote bucket to a the local `mountpoint`.
	bucket string
	// accessKey of the remote minio server.
	accessKey string
	// secretKey of the remote Minio server.
	secretKey string
}

// Represents an instance of `minfs` mount of remote Minio bucket.
// Its defined by
//   - The server info of the mount.
//   - The local mountpoint.
//   - The number of connections alive for the mount (No.Of.Services still using the mount point).
type mountInfo struct {
	config     serverConfig
	mountPoint string
	// the number of containers using the mount.
	// an active mount is done when the count is 0.
	// unmount is done only if the number of connections is 0.
	// otherwise just the count is decreased.
	connections int
}

// minfsDriver - The struct implements the `github.com/docker/go-plugins-helpers/volume.Driver` interface.
// Here are the sequence of events that defines the interaction between docker and the plugin server.
// 1. The driver implements the interface defined in `github.com/docker/go-plugins-helpers/volume.Driver`.
//    In our case the struct `minfsDriver` implements the interface.
// 2. Create a new instance of `minfsDriver` and register it with the `go-plugin-helper`.
//    `go-plugin-helper` is a tool built to make development of docker plugins easier, visit https://github.com/docker/go-plugins-helpers/.
//     The registration is done using https://godoc.org/github.com/docker/go-plugins-helpers/volume#NewHandler .
// 3. Docker interacts with the plugin server via HTTP endpoints whose
//    protocols defined here https://docs.docker.com/engine/extend/plugins_volume/#/volumedrivercreate.
// 4. Once registered the implemented methods on `minfsDriver` are called whenever docker
//    interacts with the plugin via HTTP requests. These methods are resposible for responding to docker with
//    success or error messages.
type minfsDriver struct {
	// used for atomic access to the fields.
	sync.RWMutex
	mountRoot string
	// config of the remote Minio server.
	config serverConfig
	// the local path to which the remote Minio bucket is mounted to.

	// An active volume driver server can be used to mount multiple
	// remote buckets possibly even referring to different Minio server
	// instances or buckets.
	// The state info of these mounts are maintained here.
	mounts map[string]*mountInfo
}

// return a new instance of minfsDriver.
func newMinfsDriver(mountRoot string) *minfsDriver {
	logrus.WithField("method", "new minfs driver").Debug(mountRoot)

	d := &minfsDriver{
		mountRoot: mountRoot,
		config:    serverConfig{},
		mounts:    make(map[string]*mountInfo),
	}

	return d
}

// *minfsDriver.Create - This method is called by docker when a volume is created
//                       using `$docker volume create -d <plugin-name> --name <volume-name>`.
// the name (--name) of the plugin uniquely identifies the mount.
// The name of the plugin is passed by docker to the plugin during the HTTP call, check
// https://docs.docker.com/engine/extend/plugins_volume/#/volumedrivercreate for more details.
// Additional options can be passed only during call to `Create`,
// $ docker volume create -d <plugin-name> --name <volume-name> -o <option-key>=<option-value>
// The name of the volume uniquely identifies the mount.
// The remote bucket will be mounted at `mountRoot + volumeName`.
// mountRoot is passed as `--mountroot` flag when starting the plugin server.
func (d *minfsDriver) Create(r volume.Request) volume.Response {
	logrus.WithField("method", "Create").Debugf("%#v", r)

	// hold lock for safe access.
	d.Lock()
	defer d.Unlock()

	// validate the inputs.
	// verify that the name of the volume is not empty.
	if r.Name == "" {
		return errorResponse("Name of the driver cannot be empty.Use `$ docker volume create -d <plugin-name> --name <volume-name>`")
	}

	// if the volume is already created verify that the server configs match.
	// If not return with error.
	// Since the plugin system identifies a mount uniquely by its name,
	// its not possible to create a duplicate volume pointing to a different Minio server or bucket.
	if mntInfo, ok := d.mounts[r.Name]; ok {
		// Since the volume by the given name already exists,
		// match to see whether the endpoint, bucket, accessKey
		// and secretKey of the new request and the existing entry
		// match. return error on mismatch. else return with success message,
		// Since the volume already exists no need to proceed further.
		err := matchServerConfig(mntInfo.config, r)
		if err != nil {
			return errorResponse(err.Error())
		}
		// return success since the volume exists and the configs match.
		return volume.Response{}
	}

	// verify that all the options are set when the volume is created.
	if r.Options == nil {
		return errorResponse("No options provided. Please refer example usage.")
	}
	if r.Options["endpoint"] == "" {
		return errorResponse("endpoint option cannot be empty.")
	}
	if r.Options["bucket"] == "" {
		return errorResponse("bucket option cannot be empty.")
	}
	if r.Options["access-key"] == "" {
		return errorResponse("access-key option cannot be empty")
	}
	if r.Options["secret-key"] == "" {
		return errorResponse("secret-key cannot be empty.")
	}

	mntInfo := &mountInfo{}
	config := serverConfig{}

	// Additional options passed with `-o` option are parsed here.
	config.endpoint = r.Options["endpoint"]
	config.bucket = r.Options["bucket"]
	config.secretKey = r.Options["secret-key"]
	config.accessKey = r.Options["access-key"]

	// find out whether the scheme of the URL is HTTPS.
	enableSSL, err := isSSL(config.endpoint)
	if err != nil {
		logrus.Error("Please send a valid URL of form http(s)://my-minio.com:9000 <ERROR> ", err.Error())
		return errorResponse(err.Error())
	}

	minioHost, err := getHost(config.endpoint)
	if err != nil {
		logrus.Error("Please send a valid URL of form http(s)://my-minio.com:9000 <ERROR> ", err.Error())
		return errorResponse(err.Error())
	}

	// Verify if the bucket exists.
	// If it doesnt exist create the bucket on the remote Minio server.
	// Initialize minio client object.
	minioClient, err := minio.New(minioHost, config.accessKey, config.secretKey, enableSSL)
	if err != nil {
		logrus.Errorf("Error creating new Minio client. <Error> %s", err.Error())
		return errorResponse(err.Error())
	}

	// Create a bucket.
	err = minioClient.MakeBucket(config.bucket, defaultLocation)
	if err != nil {
		// Check to see if we already own this bucket.
		if minio.ToErrorResponse(err).Code != "BucketAlreadyOwnedByYou" {
			// return with error response to docker daemon.
			logrus.WithFields(logrus.Fields{
				"endpoint": config.endpoint,
				"bucket":   config.bucket,
			}).Fatal(err.Error())
			return errorResponse(err.Error())
		}
		// bucket already exists log and return with success.
		logrus.WithFields(logrus.Fields{
			"endpoint": config.endpoint,
			"bucket":   config.bucket,
		}).Info("Bucket already exisits.")
	}

	// mountpoint is the local path where the remote bucket is mounted.
	// `mountroot` is passed as an argument while starting the server with `--mountroot` option.
	// the given bucket is mounted locally at path `mountroot + volume (r.Name is the name of
	// the volume passed by docker when a volume is created).
	mountpoint := filepath.Join(d.mountRoot, r.Name)

	// Cache the info.
	mntInfo.mountPoint = mountpoint

	// `Create` is the only function which has the abiility to pass additional options.
	// Protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedrivercreate
	// the server config info which is required for the mount later is also passed as an option during create.
	// This has to be cached for further usage.
	mntInfo.config = config

	// `r.Name` contains the plugin name passed with `--name` in
	// `$ docker volume create -d <plugin-name> --name <volume-name>`.
	// Name of the volume uniquely identifies the mount.
	d.mounts[r.Name] = mntInfo

	// ..
	return volume.Response{}
}

// minfsDriver.Remove - Delete the specified volume from disk.
// This request is issued when a user invokes `docker rm -v` to remove volumes associated with a container.
// Protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedriverremove
func (d *minfsDriver) Remove(r volume.Request) volume.Response {
	logrus.WithField("method", "remove").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.mounts[r.Name]
	// volume doesn't exist in the entry.
	// log and return error to docker daemon.
	if !ok {
		logrus.WithFields(logrus.Fields{
			"operation": "Remove",
			"volume":    r.Name,
		}).Error("Volume not found.")
		return errorResponse(fmt.Sprintf("volume %s not found", r.Name))
	}

	// The volume should be under use by any other containers.
	// verify if the number of connections is 0.
	if v.connections == 0 {
		// if the count of existing connections is 0, delete the entry for the volume.
		if err := os.RemoveAll(v.mountPoint); err != nil {
			return errorResponse(err.Error())
		}
		// Delete the entry for the mount.
		delete(d.mounts, r.Name)
		return volume.Response{}
	}

	// volume is being used by one or more containers.
	// log and return error to docker daemon.
	logrus.WithFields(logrus.Fields{
		"volume": r.Name,
	}).Errorf("Volume is currently used by %d containers. ", v.connections)

	return errorResponse(fmt.Sprintf("volume %s is currently under use.", r.Name))
}

// *minfsDriver.Path - Respond with the path on the host filesystem where the bucket mount has been made available.
// protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedriverpath
func (d *minfsDriver) Path(r volume.Request) volume.Response {
	logrus.WithField("method", "path").Debugf("%#v", r)

	d.RLock()
	defer d.RUnlock()

	v, ok := d.mounts[r.Name]
	if !ok {
		logrus.WithFields(logrus.Fields{
			"operation": "path",
			"volume":    r.Name,
		}).Error("Volume not found.")
		return errorResponse(fmt.Sprintf("volume %s not found", r.Name))
	}

	return volume.Response{Mountpoint: v.mountPoint}
}

// *minfsDriver.Mount - Does mounting of `minfs`.
// protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedrivermount
// If the mount alredy exists just increment the number of connections and return.
// Mount is called only when another container shares the created volume.
// Step 1: Create volume.
//
// $ docker volume create -d minfs-plugin \
//    --name my-test-store \
//     -o endpoint=https://play.minio.io:9000/rao -o access_key=Q3AM3UQ867SPQQA43P2F\
//     -o secret-key=zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG -o bucket=test-bucket
//
// Step 2: Attach the new volume to a new container.
//
// $ docker run -it --rm -v my-test-store:/data busybox /bin/sh
// # ls /data
//
// The above set of operations create a mount of remote bucket `test-bucket`,
// in the local path of `mountroot + my-test-store`.
// Note: mountroot passed as --mountroot flag while starting the plugin server.
func (d *minfsDriver) Mount(r volume.MountRequest) volume.Response {
	logrus.WithField("method", "mount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	// verify if the volume exists.
	// Mount operation should be performed only after creating the bucket.
	v, ok := d.mounts[r.Name]
	if !ok {
		logrus.WithFields(logrus.Fields{
			"operation": "mount",
			"volume":    r.Name,
		}).Error("method:mount: Volume not found.")
		return errorResponse(fmt.Sprintf("method:mount: volume %s not found", r.Name))
	}

	// create the directory for the mountpoint.
	// This will be the directory at which the remote bucket will be mounted.
	err := createDir(v.mountPoint)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"mountpount": v.mountPoint,
		}).Fatalf("Error creating directory for the mountpoint. <ERROR> %v.", err)
		return errorResponse(err.Error())
	}
	// If the mountpoint is already under use just increment the counter of usage and return to docker daemon.
	if v.connections > 0 {
		v.connections++
		return volume.Response{Mountpoint: v.mountPoint}
	}

	// set access-key and secret-key as env variables.
	os.Setenv("MINFS_ACCESS_KEY", v.config.accessKey)
	os.Setenv("MINFS_SECRET_KEY", v.config.secretKey)
	// Mount the remote Minio bucket to the local mountpoint.

	if err := d.mountVolume(*v); err != nil {
		logrus.WithFields(logrus.Fields{
			"mountpount": v.mountPoint,
			"endpoint":   v.config.endpoint,
			"bucket":     v.config.bucket,
		}).Fatalf("Mount failed: <ERROR> %v", err)

		return errorResponse(err.Error())
	}

	// Mount succeeds, increment the count for number of connections and return.
	v.connections++

	return volume.Response{Mountpoint: v.mountPoint}
}

// *minfsDriver.Unmount - unmounts the mount at `mountpoint`.
// protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedriverunmount
// Unmount is called when a container using the mounted volume is stopped.
func (d *minfsDriver) Unmount(r volume.UnmountRequest) volume.Response {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	// verify if the mount exists.
	v, ok := d.mounts[r.Name]
	if !ok {
		// mount doesn't exist, return error.
		logrus.WithFields(logrus.Fields{
			"operation": "unmount",
			"volume":    r.Name,
		}).Error("Volume not found.")

		return errorResponse(fmt.Sprintf("volume %s not found", r.Name))
	}

	// Unmount is done only if no other containers are using the mounted volume.
	if v.connections <= 1 {
		// unmount.
		if err := d.unmountVolume(v.mountPoint); err != nil {
			return errorResponse(err.Error())
		}
		v.connections = 0
	} else {
		// If the count is > 1, that is if the mounted volume is already being used by
		// another container, dont't unmount, just decrease the count and return.
		v.connections--
	}

	return volume.Response{}
}

// *minfsDriver.Get - Get the mount info.
// protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedriverget
func (d *minfsDriver) Get(r volume.Request) volume.Response {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	// verify if the mount exists.
	v, ok := d.mounts[r.Name]
	if !ok {
		// mount doesn't exist, return error.
		logrus.WithFields(logrus.Fields{
			"operation": "unmount",
			"volume":    r.Name,
		}).Error("Volume not found.")
		return errorResponse(fmt.Sprintf("volume %s not found", r.Name))
	}

	return volume.Response{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.mountPoint}}
}

// *minfsDriver.List - Get the list of existing volumes.
// protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedriverlist
func (d *minfsDriver) List(r volume.Request) volume.Response {
	logrus.WithField("method", "list").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	var vols []*volume.Volume
	for name, v := range d.mounts {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.mountPoint})
	}
	return volume.Response{Volumes: vols}
}

// *minfsDriver.Capabilities -  Takes values "local" or "global", more info in protocol doc below.
// protocol doc: https://docs.docker.com/engine/extend/plugins_volume/#/volumedrivercapabilities
func (d *minfsDriver) Capabilities(r volume.Request) volume.Response {
	logrus.WithField("method", "capabilities").Debugf("%#v", r)

	return volume.Response{Capabilities: volume.Capability{Scope: "local"}}
}

// mounts minfs to the local mountpoint.
func (d *minfsDriver) mountVolume(v mountInfo) error {

	// URL for the bucket (ex: https://play.minio.io:9000/mybucket).
	var bucketPath string
	if strings.HasSuffix(v.config.endpoint, "/") {
		bucketPath = v.config.endpoint + v.config.bucket
	} else {
		bucketPath = v.config.endpoint + "/" + v.config.bucket
	}

	// mount command for minfs.
	// ex:  mount -t minfs https://play.minio.io:9000/testbucket /testbucket
	cmd := fmt.Sprintf("mount -t minfs %s %s", bucketPath, v.mountPoint)

	logrus.Debug(cmd)
	return exec.Command("sh", "-c", cmd).Run()
}

// executes `unmount` on the specified volume.
func (d *minfsDriver) unmountVolume(target string) error {
	//  Unmount the volume.
	cmd := fmt.Sprintf("umount %s", target)
	logrus.Debug(cmd)
	return exec.Command("sh", "-c", cmd).Run()
}

func main() {
	// --mountroot flag defines the root folder where are the volumes are mounted.
	// If the option is not specified '/mnt' is taken as default mount root.
	mountRoot := flag.String("mountroot", "/mnt", "root for mouting Minio buckets.")
	flag.Parse()

	// check if the mount root exists.
	// create if it doesn't exist.
	err := createDir(*mountRoot)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"mountroot": mountRoot,
		}).Fatalf("Unable to create mountroot.")

		return
	}

	// if `export DEBUG=1` is set, debug logs will be printed.
	debug := os.Getenv("DEBUG")
	if ok, _ := strconv.ParseBool(debug); ok {
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Create a new instance MinfsDriver.
	// The struct implements the `github.com/docker/go-plugins-helpers/volume.Driver` interface.
	d := newMinfsDriver(*mountRoot)

	// register it with the `go-plugin-helper`.
	// `go-plugin-helper` is a tool built to make development of docker plugins easier,
	// visit https://github.com/docker/go-plugins-helpers/.
	// The registration is done using https://godoc.org/github.com/docker/go-plugins-helpers/volume#NewHandler .
	h := volume.NewHandler(d)

	// create a server on unix socket.
	logrus.Infof("listening on %s", socketAddress)
	logrus.Error(h.ServeUnix(socketAddress, 0))
}
