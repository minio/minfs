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

// Package cmd parses the parameters and runs MinFS
package cmd

import (
	"os/user"
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
  ` + Version + `{{ "\n"}}`

// NewApp initializes CLI framework for minfs.
func NewApp() *cli.App {
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
		if _, err := minfs.InitMinFSConfig(); err != nil {
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

			// Set permissions vs executing username
			user, err := user.Current()
			if err != nil {
				panic(err)
			}

			fmt.Println("Hi " + user.Name + ", you're mounting lunafs as (uid: " + user.Uid + " gid: " + user.Gid + ")")

			uidval, err := strconv.Atoi(user.Uid)
			if err != nil {
				return fmt.Errorf("Uid is not a valid value: %s", user.Uid)
			}
			opts = append(opts, minfs.SetUID(uint32(uidval)))


			gidval, err := strconv.Atoi(user.Gid)
			if err != nil {
				return fmt.Errorf("Gid is not a valid value: %s", user.Gid)
			}
			opts = append(opts, minfs.SetGID(uint32(gidval)))


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

	return app
}

// Main is the actual run function
func Main(app *cli.App, args []string) {
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

	// Run the app - exit on error.
	if err := app.Run(args); err != nil {
		log.Fatalln(err)
	}
}
