package bow

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"

	"github.com/dgraph-io/badger"

	"github.com/zippoxer/bow/codec"
	jsoncodec "github.com/zippoxer/bow/codec/json"
	keycodec "github.com/zippoxer/bow/codec/key"

	"github.com/sony/sonyflake"
)

var (
	ErrNotFound = errors.New("Record doesn't exist")
	ErrReadOnly = errors.New("Put and Delete aren't allowed in read-only mode")
)

// version increases when backwards-incompatible change is introduced,
// and Bow can't open databases created before the change.
const version = 1

// Size of bucket ids in bytes.
const bucketIdSize = 2

// MaxBuckets is the maximum amount of buckets that can be created in a database.
const MaxBuckets = math.MaxUint16 - (8 * 256)

// First byte of reserved Badger keys.
const reserved byte = 0x00

var (
	// Sequence reserved for generating bucket ids.
	bucketIdSequence = []byte{reserved, 0x00}

	// Key reserved for metadata.
	metaKey = []byte{reserved, 0x01}
)

// Dependencies.
var (
	// Encoding and decoding of keys.
	keyCodec = keycodec.Codec{}

	// Random key generator.
	sonyflakeKeygen = sonyflake.NewSonyflake(sonyflake.Settings{
		MachineID: func() (uint16, error) {
			// We don't need machine ID since Bow isn't distributed.
			// Instead, we return 2 random bytes to increase entropy.
			return uint16(rand.Uint32() & (1<<16 - 1)), nil
		},
	})
)

// Id is a convenient type for randomly generated keys.
type Id string

// NewId generates an 8-byte unique Id.
func NewId() Id {
	id, err := sonyflakeKeygen.NextID()
	if err != nil {
		panic(fmt.Sprintf("bow.NewId: %v", err))
	}
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, id)
	return Id(b)
}

// ParseId parses the user-friendly output of String to an Id.
func ParseId(s string) (Id, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	if len(b) != 8 {
		return "", fmt.Errorf("bow.ParseId: input must be exactly 8 bytes long")
	}
	return Id(b), nil
}

// String returns a user-friendly format of the Id.
func (id Id) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

func (id Id) MarshalJSON() ([]byte, error) {
	s := id.String()
	b := make([]byte, len(s)+2)
	b[0] = '"'
	copy(b[1:], s)
	b[len(b)-1] = '"'
	return b, nil
}

func (id *Id) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	*id, err = ParseId(s)
	return err
}

func (id Id) Marshal(in []byte) ([]byte, error) {
	return []byte(id), nil
}

func (id *Id) Unmarshal(b []byte) error {
	*id = Id(b)
	return nil
}

// Option is a function that configures a DB.
type Option func(db *DB) error

func SetReadOnly(readOnly bool) Option {
	return func(db *DB) error {
		db.readOnly = readOnly
		return nil
	}
}

func SetCodec(c codec.Codec) Option {
	return func(db *DB) error {
		db.codec = c
		return nil
	}
}

func SetBadgerOptions(o badger.Options) Option {
	return func(db *DB) error {
		db.badgerOptions = o
		return nil
	}
}

// DB is an opened Bow database.
type DB struct {
	db       *badger.DB
	meta     meta
	metaMu   sync.RWMutex
	bucketId *badger.Sequence

	readOnly      bool
	codec         codec.Codec
	badgerOptions badger.Options
}

// Open opens a database at the given directory. If the directory doesn't exist,
// then it will be created.
//
// Configure the database by passing the result of functions like SetCodec or
// SetBadgerOptions.
//
// Make sure to call Close after you're done.
func Open(dir string, options ...Option) (*DB, error) {
	db := &DB{
		badgerOptions: badger.DefaultOptions(dir),
		codec:         jsoncodec.Codec{},
	}

	// Apply options.
	for _, option := range options {
		err := option(db)
		if err != nil {
			return nil, err
		}
	}

	// Sync db.readOnly with db.badgerOptions.ReadOnly
	if db.readOnly || db.badgerOptions.ReadOnly {
		db.readOnly = true
		db.badgerOptions.ReadOnly = true
	}

	// Propagate options down to badgerOptions.
	if db.badgerOptions.Dir == "" {
		db.badgerOptions.Dir = dir
	}
	if db.badgerOptions.ValueDir == "" {
		db.badgerOptions.ValueDir = dir
	}

	bdb, err := badger.Open(db.badgerOptions)
	if err != nil {
		return nil, err
	}
	db.db = bdb

	err = db.readMeta(nil)
	if err == badger.ErrKeyNotFound {
		db.meta = meta{
			Version: version,
			Buckets: make(map[string]bucketMeta),
		}
		if !db.readOnly {
			err = db.writeMeta(nil)
			if err != nil {
				return nil, err
			}
		}
	} else if err != nil {
		return nil, err
	}

	if !db.readOnly {
		db.bucketId, err = db.db.GetSequence(bucketIdSequence, 1e3)
		if err != nil {
			return nil, err
		}
	}

	return db, nil
}

// Bucket returns the named bucket, creating it if it doesn't exist.
// If an error has occurred during creation, it would be returned by
// any operation on the returned bucket.
func (db *DB) Bucket(name string) *Bucket {
	bucket, ok := db.bucket(name)
	if !ok {
		if db.readOnly {
			return &Bucket{err: ErrNotFound}
		}
		bucket, err := db.createBucket(nil, name)
		if err != nil {
			return &Bucket{err: err}
		}
		return bucket
	}
	return bucket
}

// Buckets returns a list of the names of all the buckets in the DB.
func (db *DB) Buckets() []string {
	db.metaMu.RLock()
	defer db.metaMu.RUnlock()
	names := make([]string, 0, len(db.meta.Buckets))
	for name := range db.meta.Buckets {
		names = append(names, name)
	}
	return names
}

// Badger exposes the internal Badger database.
// Use it to call Backup, Load or RunValueLogGC.
// Do NOT perform Set operations as you may corrupt Bow.
func (db *DB) Badger() *badger.DB {
	return db.db
}

// Close releases all database resources.
func (db *DB) Close() error {
	if db.bucketId != nil {
		err := db.bucketId.Release()
		if err != nil {
			return err
		}
	}
	return db.db.Close()
}

func (db *DB) bucket(name string) (*Bucket, bool) {
	db.metaMu.RLock()
	meta, ok := db.meta.Buckets[name]
	db.metaMu.RUnlock()
	if !ok {
		return nil, false
	}
	bucket := &Bucket{
		db: db,
		id: meta.Id,
	}
	return bucket, true
}

func (db *DB) createBucket(txn *badger.Txn, name string) (*Bucket, error) {
	db.metaMu.Lock()
	defer db.metaMu.Unlock()

	meta, ok := db.meta.Buckets[name]
	if ok {
		return &Bucket{db: db, id: meta.Id}, nil
	}

	nextId, err := db.bucketId.Next()
	if err != nil {
		return nil, err
	}
	// This increments the first byte of the bucket id by 8. The bucket id
	// prefixes records in the database, and since values 0 to 8 of the
	// first byte of keys are reserved for internal use, bucket ids can't
	// have their first byte between 0 and 8.
	nextId += 8 * 256
	if nextId > MaxBuckets {
		return nil, fmt.Errorf("bow.createBucket: reached maximum amount of buckets limit (%d)",
			MaxBuckets)
	}

	var id bucketId
	binary.BigEndian.PutUint16(id[:], uint16(nextId))
	db.meta.Buckets[name] = bucketMeta{
		Id: id,
	}
	err = db.writeMeta(txn)
	if err != nil {
		return nil, err
	}

	return &Bucket{db: db, id: id}, err
}

func (db *DB) readMeta(txn *badger.Txn) error {
	if txn == nil {
		txn = db.db.NewTransaction(false)
		defer func() {
			txn.Discard()
		}()
	}
	item, err := txn.Get(metaKey)
	if err != nil {
		return err
	}
	return item.Value(func(value []byte) error {
		return json.Unmarshal(value, &db.meta)
	})
}

func (db *DB) writeMeta(txn *badger.Txn) (err error) {
	if txn == nil {
		txn = db.db.NewTransaction(true)
		defer func() {
			err = txn.Commit()
		}()
	}
	b, err := json.Marshal(db.meta)
	if err != nil {
		return err
	}
	err = txn.Set(metaKey, b)
	return
}

type bucketMeta struct {
	Id bucketId
}

type meta struct {
	Version uint32
	Buckets map[string]bucketMeta
}
