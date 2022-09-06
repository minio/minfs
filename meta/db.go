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

// Package meta maintains the caching of all meta data of the files and directories.
package meta

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/vmihailenco/msgpack.v2"

	minio "github.com/minio/minio-go/v7"
	"go.etcd.io/bbolt"
)

// RegisterExt -
func RegisterExt(id int8, value interface{}) interface{} {
	msgpack.RegisterExt(id, value)
	return value
}

// Open -
func Open(path string, mode os.FileMode, options *bbolt.Options) (*DB, error) {
	dname := filepath.Dir(path)
	if err := os.MkdirAll(dname, 0700); err != nil {
		return nil, err
	}
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	return &DB{
		db,
	}, nil

}

// DB -
type DB struct {
	*bbolt.DB
}

// Begin -
func (db *DB) Begin(writable bool) (*Tx, error) {
	tx, err := db.DB.Begin(writable)
	return &Tx{tx}, err
}

// Update -
func (db *DB) Update(fn func(*Tx) error) error {
	return db.DB.Update(func(tx *bbolt.Tx) error {
		return fn(&Tx{tx})
	})
}

// View -
func (db *DB) View(fn func(*Tx) error) error {
	return db.DB.View(func(tx *bbolt.Tx) error {
		return fn(&Tx{tx})
	})
}

// Bucket -
type Bucket struct {
	InnerBucket *bbolt.Bucket
}

// Bucket -
func (b *Bucket) Bucket(name string) *Bucket {
	return &Bucket{
		b.InnerBucket.Bucket([]byte(name)),
	}
}

// NextSequence -
func (b *Bucket) NextSequence() (uint64, error) {
	return b.InnerBucket.NextSequence()
}

// ForEach -
func (b *Bucket) ForEach(fn func(string, interface{}) error) error {
	return b.InnerBucket.ForEach(func(k, v []byte) error {
		if k[len(k)-1] == '/' {
			return nil
		}

		var o interface{}
		if err := msgpack.Unmarshal(v, &o); err != nil {
			return err
		}

		return fn(string(k), o)
	})
}

// CreateBucketIfNotExists -
func (b *Bucket) CreateBucketIfNotExists(key string) (*Bucket, error) {
	child, err := b.InnerBucket.CreateBucketIfNotExists([]byte(key))
	return &Bucket{child}, err
}

// Tx - transaction struct.
type Tx struct {
	*bbolt.Tx
}

// Bucket -
func (tx *Tx) Bucket(name string) *Bucket {
	return &Bucket{
		tx.Tx.Bucket([]byte(name)),
	}
}

// ErrNoSuchObject - returned when object is not found.
var ErrNoSuchObject = errors.New("No such object")

// IsNoSuchObject - is err ErrNoSuchObject ?
func IsNoSuchObject(err error) bool {
	if err == nil {
		return false
	}
	// Validate if the type is same as well.
	if err == ErrNoSuchObject {
		return true
	} else if err.Error() == ErrNoSuchObject.Error() {
		// Reaches here when type did not match but err string matches.
		// Someone wrapped this error? - still return true since
		// they are the same.
		return true
	}
	errorResponse := minio.ToErrorResponse(err)
	return errorResponse.Code == "NoSuchKey"
}

// DeleteBucket -
func (b *Bucket) DeleteBucket(key string) error {
	return b.InnerBucket.DeleteBucket([]byte(key))
}

// Delete -
func (b *Bucket) Delete(key string) error {
	return b.InnerBucket.Delete([]byte(key))
}

// Get -
func (b *Bucket) Get(key string, v ...interface{}) error {
	data := b.InnerBucket.Get([]byte(key))
	if data == nil {
		return ErrNoSuchObject
	}
	return msgpack.Unmarshal(data, v...)
}

// Put -
func (b *Bucket) Put(key string, v interface{}) error {
	data, err := msgpack.Marshal(v)
	if err != nil {
		return err
	}
	return b.InnerBucket.Put([]byte(key), data)
}
