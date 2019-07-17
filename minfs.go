// +build go1.12

/*
 * MinFS - fuse driver for Object Storage (C) 2016, 2017 MinIO, Inc.
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

package main // import "github.com/minio/minfs"

import (
	"log"
	"os"

	minfs "github.com/minio/minfs/cmd"
	daemon "github.com/sevlyar/go-daemon"
)

func main() {
	dctx := &daemon.Context{
		PidFileName: "/var/log/minfs.pid",
		PidFilePerm: 0644,
		LogFileName: "/var/log/minfs.log",
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       027,
		Args:        os.Args,
	}

	d, err := dctx.Reborn()
	if err != nil {
		log.Fatalln("Unable to run: ", err)
	}
	if d != nil {
		return
	}
	defer dctx.Release()

	// daemon business logic starts here
	minfs.Main(os.Args)
}
