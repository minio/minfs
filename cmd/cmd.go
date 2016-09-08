/*
 * MinFS for Amazon S3 Compatible Cloud Storage (C) 2015, 2016 Minio, Inc.
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

// Package cmd parses the parameters and runs MinFS
package cmd

import (
	"flag"
	"strconv"
	"strings"

	"github.com/minio/cli"
	"github.com/minio/mc/pkg/console"
	minfs "github.com/minio/minfs/fs"
)

var (
	// global flags for minfs.
	minfsFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "help, h",
			Usage: "Show help.",
		},
	}
)

// Collection of minio flags currently supported.
var globalFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "options, o",
		Usage: "Fuse mount options.",
	},
}

// Help template for minfs.
var minfsHelpTemplate = `NAME:
  {{.Name}} - {{.Usage}}

DESCRIPTION:
  {{.Description}}

USAGE:
  {{.Name}} {{if .Flags}}[flags] {{end}}command{{if .Flags}}{{end}} [arguments...]

COMMANDS:
  {{range .Commands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
  {{end}}{{if .Flags}}
FLAGS:
  {{range .Flags}}{{.}}
  {{end}}{{end}}
VERSION:
  ` + Version +
	`{{ "\n"}}`

// Main is the actual run function
func Main() {
	// Enable profiling supported modes are [cpu, mem, block].
	/*
		switch os.Getenv("MINFS_PROFILER") {
		case "cpu":
			defer profile.Start(profile.CPUProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
		case "mem":
			defer profile.Start(profile.MemProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
		case "block":
			defer profile.Start(profile.BlockProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
		}
	*/

	// Options:
	// -- debug
	// -- bucket
	// -- target
	// -- permissions
	// -- uid / gid

	// Set up app.
	app := cli.NewApp()
	app.Name = "minfs"
	app.Author = "Minio.io"
	app.Usage = "Fuse driver for Cloud Storage Server."
	app.Description = `MinFS is a fuse driver for Amazon S3 compatible object storage server. Use it to store photos, videos, VMs, containers, log files, or any blob of data as objects on your object storage server.`
	app.Flags = append(minfsFlags, globalFlags...)
	app.CustomAppHelpTemplate = minfsHelpTemplate
	app.Before = func(c *cli.Context) error {
		if !c.IsSet("options") {
			cli.ShowAppHelpAndExit(c, 1)
		}
		return nil
	}
	app.Action = func(c *cli.Context) {
		opts := []func(*minfs.Config){}
		for _, option := range strings.Split(c.String("options"), ",") {
			vals := strings.Split(option, "=")
			switch vals[0] {
			case "uid":
				if len(vals) == 1 {
					console.Fatalln("Uid has no value")
				} else if val, err := strconv.Atoi(vals[1]); err != nil {
					console.Fatalf("Uid is not a valid value: %s\n", vals[1])
				} else {
					opts = append(opts, minfs.SetUID(uint32(val)))
				}
			case "gid":
				if len(vals) == 1 {
					console.Fatalln("Uid has no value")
				} else if val, err := strconv.Atoi(vals[1]); err != nil {
					console.Fatalf("Gid is not a valid value: %s\n", vals[1])
				} else {
					opts = append(opts, minfs.SetGID(uint32(val)))
				}
			case "cache":
				if len(vals) == 1 {
					console.Fatalln("Cache has no value")
				} else {
					opts = append(opts, minfs.CacheDir(vals[1]))
				}
			}

			target := flag.Arg(0)
			mountpoint := flag.Arg(1)

			opts = append(opts, minfs.Mountpoint(mountpoint), minfs.Target(target), minfs.Debug())
			fs, err := minfs.New(opts...)
			if err != nil {
				console.Fatalln("Unable to instantiate a new minfs", err)
			}
			err = fs.Serve()
			if err != nil {
				console.Fatalln("Unable to serve a minfs", err)
			}
		}
	}

	// Run the app - exit on error.
	app.RunAndExitOnError()

}
