package meta

// Meta package maintains the caching of all meta data of the files and directories.

import (
	"errors"
	"log"
	"os"

	"gopkg.in/vmihailenco/msgpack.v2"

	"github.com/boltdb/bolt"
)

func RegisterExt(id int8, value interface{}) interface{} {
	msgpack.RegisterExt(id, value)
	return value
}

func Open(path string, mode os.FileMode, options *bolt.Options) (*DB, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	return &DB{
		db,
	}, nil

}

type DB struct {
	*bolt.DB
}

func (db *DB) Begin(writable bool) (*Tx, error) {
	tx, err := db.DB.Begin(writable)
	return &Tx{tx}, err
}

func (db *DB) View(fn func(*Tx) error) error {
	return db.DB.View(func(tx *bolt.Tx) error {
		return fn(&Tx{tx})
	})
}

type Bucket struct {
	InnerBucket *bolt.Bucket
}

func (b *Bucket) Bucket(name string) *Bucket {
	return &Bucket{
		b.InnerBucket.Bucket([]byte(name)),
	}
}

func (b *Bucket) NextSequence() (uint64, error) {
	return b.InnerBucket.NextSequence()
}

func (b *Bucket) ForEach(fn func(string, interface{}) error) error {

	return b.InnerBucket.ForEach(func(k, v []byte) error {
		var o interface{}
		if err := msgpack.Unmarshal(v, &o); err != nil {
			return err
		}

		return fn(string(k), o)
	})
}

func (b *Bucket) CreateBucketIfNotExists(key string) (*Bucket, error) {
	child, err := b.InnerBucket.CreateBucketIfNotExists([]byte(key))
	return &Bucket{child}, err
}

type Tx struct {
	*bolt.Tx
}

func (tx *Tx) Bucket(name string) *Bucket {
	return &Bucket{
		tx.Tx.Bucket([]byte(name)),
	}
}

var ErrNoSuchObject = errors.New("No such object.")

func IsNoSuchObject(err error) bool {
	return err == ErrNoSuchObject
}

func (b *Bucket) DeleteBucket(key string) error {
	return b.InnerBucket.DeleteBucket([]byte(key))
}

func (b *Bucket) Delete(key string) error {
	return b.InnerBucket.Delete([]byte(key))
}

func (b *Bucket) Get(key string, v ...interface{}) error {
	data := b.InnerBucket.Get([]byte(key))
	if data == nil {
		return ErrNoSuchObject
	}

	return msgpack.Unmarshal(data, v...)
}

func (b *Bucket) Put(key string, v interface{}) error {
	if data, err := msgpack.Marshal(v); err != nil {
		return err
	} else {
		return b.InnerBucket.Put([]byte(key), data)
	}
}
