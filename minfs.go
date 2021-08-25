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
)

func main() {
	app := minfs.NewApp()
	if len(os.Args) == 1 || (len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "--version" ||
		os.Args[1] == "-h" || os.Args[1] == "-v")) {
		if err := app.Run(os.Args); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Run our app
	minfs.Main(app, os.Args)
}
