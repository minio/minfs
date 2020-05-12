/*
 * MinFS - fuse driver for Object Storage (C) 2016 MinIO, Inc.
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
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/minio/cli"
	minfs "github.com/minio/minfs/fs"
)

var (
	// global flags for minfs.
	minfsFlags = []cli.Flag{}
)

// Collection of minio flags currently supported.
var globalFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "o",
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
{{if .Commands}}
COMMANDS:
  {{range .Commands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
  {{end}}{{end}}{{if .Flags}}
FLAGS:
  {{range .Flags}}{{.}}
  {{end}}{{end}}
VERSION:
  ` + Version +
	`{{ "\n"}}` +
	`
COMMITID:
  ` + CommitID +
	`{{ "\n"}}`

// Main is the actual run function
func Main(args []string) {
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
	cli.HelpFlag = cli.BoolFlag{
		Name:  "help, h",
		Usage: "show help",
	}

	app := cli.NewApp()
	app.HideHelpCommand = true
	app.Name = "minfs"
	app.Author = "min.io"
	app.Version = Version
	app.Usage = "Fuse driver for Cloud Storage Server."
	app.Description = `MinFS is a fuse driver for MinIO server.`
	app.Flags = append(minfsFlags, globalFlags...)
	app.CustomAppHelpTemplate = minfsHelpTemplate
	app.Before = func(c *cli.Context) error {
		_, err := minfs.InitMinFSConfig()
		if err != nil {
			return fmt.Errorf("Unable to initialize minfs config %s", err)
		}
		if !c.Args().Present() {
			cli.ShowAppHelpAndExit(c, 1)
		}
		return nil
	}
	app.Action = func(c *cli.Context) error {
		opts := []func(*minfs.Config){}
		for _, option := range strings.Split(c.String("o"), ",") {
			vals := strings.Split(option, "=")
			switch vals[0] {
			case "uid":
				if len(vals) == 1 {
					return errors.New("Uid has no value")
				}
				val, err := strconv.Atoi(vals[1])
				if err != nil {
					return fmt.Errorf("Uid is not a valid value: %s", vals[1])
				}
				opts = append(opts, minfs.SetUID(uint32(val)))
			case "gid":
				if len(vals) == 1 {
					return errors.New("Gid has no value")
				}
				val, err := strconv.Atoi(vals[1])
				if err != nil {
					return fmt.Errorf("Gid is not a valid value: %s", vals[1])
				}
				opts = append(opts, minfs.SetGID(uint32(val)))
			case "cache":
				if len(vals) == 1 {
					return errors.New("Cache has no value")
				}
				opts = append(opts, minfs.CacheDir(vals[1]))
			case "insecure":
				opts = append(opts, minfs.Insecure())
			case "debug":
				opts = append(opts, minfs.Debug())
			}

			target := c.Args().Get(0)
			mountpoint := c.Args().Get(1)

			opts = append(opts, minfs.Mountpoint(mountpoint), minfs.Target(target))
		}

		fs, err := minfs.New(opts...)
		if err != nil {
			return fmt.Errorf("Unable to initialize minfs %s", err)
		}

		err = fs.Serve()
		if err != nil {
			return fmt.Errorf("Unable to serve minfs %s", err)
		}

		return nil
	}

	// Run the app - exit on error.
	if err := app.Run(args); err != nil {
		log.Fatalln(err)
	}
}
