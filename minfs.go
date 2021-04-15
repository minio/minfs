// Copyright (c) 2021 MinIO, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

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
