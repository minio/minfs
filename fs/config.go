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

package minfs

import (
	"errors"
	"net/url"
	"os"
)

// Config is being used for storge of configuration items
type Config struct {
	bucket   string
	basePath string

	cache      string
	cacheSize  uint64
	accountID  string
	target     *url.URL
	mountpoint string
	debug      bool

	uid  uint32
	gid  uint32
	mode os.FileMode
}

// Bucket option for Config
func Bucket(name string) func(*Config) {
	return func(cfg *Config) {
		cfg.bucket = name
	}
}

// Mountpoint configures the target mountpoint
func Mountpoint(mountpoint string) func(*Config) {
	return func(cfg *Config) {
		cfg.mountpoint = mountpoint
	}
}

// BasePath configures the root of the mounted bucket
func BasePath(path string) func(*Config) {
	return func(cfg *Config) {
		cfg.basePath = path
	}
}

// Bucket option for Config
func Target(target string) func(*Config) {
	return func(cfg *Config) {
		if u, err := url.Parse(target); err == nil {
			cfg.target = u

			if len(u.Path) > 0 {
				cfg.bucket = u.Path[1:]
			}
		}
	}
}

// Bucket option for Config
func CacheDir(path string) func(*Config) {
	return func(cfg *Config) {
		cfg.cache = path
	}
}

// Bucket option for Config
func CacheSize(size uint64) func(*Config) {
	return func(cfg *Config) {
		cfg.cacheSize = size
	}
}

// Gid Bucket option for Config
func Gid(gid uint32) func(*Config) {
	return func(cfg *Config) {
		cfg.gid = gid
	}
}

// Uid Bucket option for Config
func Uid(uid uint32) func(*Config) {
	return func(cfg *Config) {
		cfg.uid = uid
	}
}

func Debug() func(*Config) {
	return func(cfg *Config) {
		cfg.debug = true
	}
}

func (cfg *Config) validate() error {
	// check if mountpoint exists
	if cfg.mountpoint == "" {
		return errors.New("Mountpoint not set")
	}

	if cfg.target == nil {
		return errors.New("Target not set")
	}

	if cfg.bucket == "" {
		return errors.New("Bucket not set")
	}

	return nil
}
