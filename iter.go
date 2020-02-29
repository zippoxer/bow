package bow

import (
	"runtime"

	"github.com/dgraph-io/badger/v2"
)

type Iter struct {
	bucket     *Bucket
	prefix     []byte
	txn        *badger.Txn
	it         *badger.Iterator
	resultType *structType
	advanced   bool
	closed     bool
	err        error
}

func newIter(bucket *Bucket, prefix []byte) *Iter {
	prefix = bucket.internalKey(prefix)
	opts := badger.DefaultIteratorOptions
	opts.PrefetchSize = runtime.GOMAXPROCS(-1)
	txn := bucket.db.db.NewTransaction(false)
	it := txn.NewIterator(opts)
	it.Seek(prefix)
	return &Iter{
		bucket: bucket,
		txn:    txn,
		it:     it,
		prefix: prefix,
	}
}

func (it *Iter) Next(result interface{}) bool {
	if it.err != nil {
		return false
	}
	if it.closed {
		return false
	}
	if it.advanced {
		it.it.Next()
	}
	if !it.it.ValidForPrefix(it.prefix) {
		it.Close()
		return false
	}
	item := it.it.Item()
	ik := item.Key()
	err := item.Value(func(v []byte) error {
		var err error
		if it.resultType == nil {
			it.resultType, err = newStructType(result, true)
			if err != nil {
				return err
			}
		}
		err = it.bucket.db.codec.Unmarshal(v, result)
		if err != nil {
			return err
		}
		err = it.resultType.value(result).setKey(ik[bucketIdSize:])
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		it.err = err
		return false
	}

	if !it.advanced {
		it.advanced = true
	}
	return true
}

// Err returns the error, if any, that was encountered during iteration.
// Err may be called after an explicit or implicit Close.
func (it *Iter) Err() error {
	return it.err
}

// Close closes the Iter. If Next is called and returns false and there are no
// further results, Iter is closed automatically and it will suffice to check the
// result of Err.
func (it *Iter) Close() {
	if it.err != nil {
		return
	}
	it.closed = true
	it.it.Close()
	it.txn.Discard()
}
