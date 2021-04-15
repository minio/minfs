// This file is part of MinFS
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
