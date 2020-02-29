package bow

import (
	"github.com/dgraph-io/badger/v2"
)

type bucketId [bucketIdSize]byte

// Bucket represents a collection of records in the database.
type Bucket struct {
	id  bucketId
	db  *DB
	err error
}

// Put persists a record into the bucket. If a record with the same key already
// exists, then it will be updated.
func (b *Bucket) Put(v interface{}) error {
	if b.db.readOnly {
		return ErrReadOnly
	}
	if b.err != nil {
		return b.err
	}
	typ, err := newStructType(v, false)
	if err != nil {
		return err
	}
	key, err := typ.value(v).key()
	if err != nil {
		return err
	}
	data, err := b.db.codec.Marshal(v, nil)
	if err != nil {
		return err
	}
	return b.PutBytes(key, data)
}

func (b *Bucket) PutBytes(key interface{}, data []byte) error {
	if b.db.readOnly {
		return ErrReadOnly
	}
	if b.err != nil {
		return b.err
	}
	keyBytes, err := keyCodec.Marshal(key, nil)
	if err != nil {
		return err
	}
	var ik []byte
	if len(keyBytes) == 0 {
		ik = b.internalKey([]byte(NewId()))
	} else {
		ik = b.internalKey(keyBytes)
	}
	return b.db.db.Update(func(txn *badger.Txn) error {
		return txn.Set(ik, data)
	})
}

// Get retrieves a record from the bucket by key, returning ErrNotFound if
// it doesn't exist.
func (b *Bucket) Get(key interface{}, v interface{}) error {
	if b.err != nil {
		return b.err
	}
	keyBytes, err := keyCodec.Marshal(key, nil)
	if err != nil {
		return err
	}
	ik := b.internalKey(keyBytes)
	typ, err := newStructType(v, true)
	if err != nil {
		return err
	}
	typ.value(v).setKey(keyBytes)
	return b.db.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(ik)
		if err == badger.ErrKeyNotFound {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error {
			return b.db.codec.Unmarshal(value, v)
		})
	})
}

func (b *Bucket) GetBytes(key interface{}, in []byte) (out []byte, err error) {
	if b.err != nil {
		return nil, b.err
	}
	keyBytes, err := keyCodec.Marshal(key, nil)
	if err != nil {
		return nil, err
	}
	ik := b.internalKey(keyBytes)
	err = b.db.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(ik)
		if err == badger.ErrKeyNotFound {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error {
			size := len(value)
			if size == 0 {
				return nil
			}
			if size > cap(in) {
				in = make([]byte, size)
			}
			copy(in, value)
			out = in[:size]
			return nil
		})
	})
	return
}

// Delete removes a record from the bucket by key.
func (b *Bucket) Delete(key interface{}) error {
	if b.db.readOnly {
		return ErrReadOnly
	}
	if b.err != nil {
		return b.err
	}
	keyBytes, err := keyCodec.Marshal(key, nil)
	if err != nil {
		return err
	}
	ik := b.internalKey(keyBytes)
	return b.db.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(ik)
	})
}

// Iter returns an iterator for all the records in the bucket.
func (b *Bucket) Iter() *Iter {
	if b.err != nil {
		return &Iter{err: b.err}
	}
	iter := newIter(b, nil)
	return iter
}

// Prefix returns an iterator for all the records whose key has the given prefix.
func (b *Bucket) Prefix(prefix interface{}) *Iter {
	if b.err != nil {
		return &Iter{err: b.err}
	}
	key, err := keyCodec.Marshal(prefix, nil)
	if err != nil {
		return &Iter{err: err}
	}
	iter := newIter(b, key)
	return iter
}

// internalKey returns key prefixed with the bucket's id.
func (b *Bucket) internalKey(key []byte) []byte {
	buf := make([]byte, len(key)+bucketIdSize)
	copy(buf, b.id[:])
	copy(buf[bucketIdSize:], key)
	return buf
}
