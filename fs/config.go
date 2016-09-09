/*
 * MinFS - fuse driver for Object Storage (C) 2016 Minio, Inc.
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
	"path"
	"strings"
)

// Config is being used for storge of configuration items
type Config struct {
	bucket   string
	basePath string

	cache      string
	accountID  string
	target     *url.URL
	mountpoint string
	debug      bool

	uid  uint32
	gid  uint32
	mode os.FileMode
}

// Mountpoint configures the target mountpoint
func Mountpoint(mountpoint string) func(*Config) {
	return func(cfg *Config) {
		cfg.mountpoint = mountpoint
	}
}

// Target url target option for Config
func Target(target string) func(*Config) {
	return func(cfg *Config) {
		if u, err := url.Parse(target); err == nil {
			cfg.target = u

			parts := strings.Split(u.Path[1:], "/")

			if len(parts) >= 0 {
				cfg.bucket = parts[0]
			}

			if len(parts) >= 1 {
				cfg.basePath = path.Join(parts[1:]...)
			}
		}
	}
}

// CacheDir - cache directory path option for Config
func CacheDir(path string) func(*Config) {
	return func(cfg *Config) {
		cfg.cache = path
	}
}

// SetGID - sets a custom gid for the mount.
func SetGID(gid uint32) func(*Config) {
	return func(cfg *Config) {
		cfg.gid = gid
	}
}

// SetUID - sets a custom uid for the mount.
func SetUID(uid uint32) func(*Config) {
	return func(cfg *Config) {
		cfg.uid = uid
	}
}

// Debug - enables debugging.
func Debug() func(*Config) {
	return func(cfg *Config) {
		cfg.debug = true
	}
}

// Validates the config for sane values.
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
