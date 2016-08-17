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
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
	minfs "github.com/minio/minfs/fs"
)

var options = flag.String("o", "", "mount options")

func usage() {
	fmt.Fprintf(os.Stderr, "MinFS for cloud storage and filesystems.\n\n")

	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", os.Args[0])
	flag.PrintDefaults()
}

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

	// arguments:
	// -- debug
	// -- bucket
	// -- target
	// -- permissions
	// -- uid / gid

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 2 {
		usage()
		os.Exit(2)
	}

	opts := []func(*minfs.Config){}

	for _, option := range strings.Split(*options, ",") {
		vals := strings.Split(option, "=")

		switch vals[0] {
		case "uid":
			if len(vals) == 1 {
				fmt.Fprint(os.Stderr, color.RedString("Uid has no value\n"))
				os.Exit(2)
			} else if val, err := strconv.Atoi(vals[1]); err != nil {
				fmt.Fprint(os.Stderr, color.RedString("Uid is not a valid value: %s\n", vals[1]))
				os.Exit(2)
			} else {
				opts = append(opts, minfs.Uid(uint32(val)))
			}
		case "gid":
			if len(vals) == 1 {
				fmt.Fprint(os.Stderr, color.RedString("Uid has no value\n"))
				os.Exit(2)
			} else if val, err := strconv.Atoi(vals[1]); err != nil {
				fmt.Fprint(os.Stderr, color.RedString("Gid is not a valid value: %s\n", vals[1]))
				os.Exit(2)
			} else {
				opts = append(opts, minfs.Gid(uint32(val)))
			}
		case "cache":
			if len(vals) == 1 {
				fmt.Fprint(os.Stderr, color.RedString("Cache has no value\n"))
				os.Exit(2)
			} else {
				opts = append(opts, minfs.CacheDir(vals[1]))
			}
		}
	}

	target := flag.Arg(0)
	mountpoint := flag.Arg(1)

	if fs, err := minfs.New(
		append(opts,
			minfs.Mountpoint(mountpoint),
			minfs.Target(target),
			minfs.Debug(),
		)...,
	); err != nil {
		panic(err)
	} else if err := fs.Serve(); err != nil {
		panic(err)
	}
}
