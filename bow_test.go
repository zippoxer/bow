package bow

import (
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

type Arrow struct {
	Id        string `bow:"key"`
	Length    int
	Sharpness float64
}

type Quiver struct {
	Id     int `bow:"key"`
	Arrows []Arrow
}

type Armory struct {
	Id      Id
	Quivers []Quiver
}

// Tests Put, Get and Iter on multiple buckets.
func Test(t *testing.T) {
	db := OpenTestDB(t)
	defer db.Drop()

	a1 := Arrow{Id: "123", Length: 10, Sharpness: 0.97}
	db.Put("arrows", a1)
	var got Arrow
	db.Get("arrows", a1.Id, &got)
	if !reflect.DeepEqual(a1, got) {
		t.Fatalf("expected %v, got %v", a1, got)
	}

	// Put with same id, should update.
	a2 := Arrow{Id: a1.Id, Length: 8, Sharpness: 0.98}
	db.Put("arrows", a2)
	db.Get("arrows", a1.Id, &got)
	if !reflect.DeepEqual(a2, got) {
		t.Fatal("object not updated")
	}

	// Put with another id, should insert.
	a3 := Arrow{Id: "456", Length: 5, Sharpness: 1.00}
	db.Put("arrows", a3)
	db.Get("arrows", "456", &got)
	if !reflect.DeepEqual(a3, got) {
		t.Fatal("object not inserted")
	}

	// Put with same id in another bucket.
	db.Put("new_arrows", a2)
	db.Get("new_arrows", a2.Id, &got)
	if !reflect.DeepEqual(a2, got) {
		t.Fatal("object not inserted")
	}

	// Make sure the first object is still OK.
	db.Get("arrows", "123", &got)
	if !reflect.DeepEqual(a2, got) {
		t.Fatal("object changed for no reason")
	}

	// Re-open the database.
	db.Close()
	db.Open()

	// Make sure we got all the buckets.
	if !reflect.DeepEqual(db.DB().Buckets(), []string{"arrows", "new_arrows"}) {
		t.Fatalf("lost/gained buckets after re-opening: %v", db.DB().Buckets())
	}

	// Iterate a bucket and make sure it contains what we put in it before.
	iter := db.DB().Bucket("arrows").Iter()
	defer iter.Close()
	found := map[Arrow]bool{
		a2: false,
		a3: false,
	}
	for iter.Next(&got) {
		_, ok := found[got]
		if !ok {
			t.Fatalf("got object which shouldn't exist: %v", got)
		}
		if found[got] {
			t.Fatalf("got same object twice: %v", got)
		}
		found[got] = true
	}
	if iter.Err() != nil {
		t.Fatal(iter.Err())
	}
	for a, ok := range found {
		if !ok {
			t.Fatalf("didn't find object: %v", a)
		}
	}
}

// Tests Put while in Iter.
func TestIterPut(t *testing.T) {
	db := OpenTestDB(t)
	defer db.Drop()

	a1 := Arrow{Id: "123", Length: 10, Sharpness: 0.97}
	a2 := Arrow{Id: "456", Length: 15, Sharpness: 0.98}
	db.Put("arrows", a1)
	db.Put("arrows", a2)

	iter := db.DB().Bucket("arrows").Iter()
	defer iter.Close()
	var got Arrow
	if !iter.Next(&got) {
		t.Fatal("no results")
	}
	newA1 := Arrow{
		Id:     a1.Id,
		Length: 20,
	}
	db.Put("arrows", newA1)
	if !iter.Next(&got) {
		t.Fatal("no 2nd result")
	}
	if iter.Next(&got) {
		t.Fatal("too many results")
	}
	if iter.Err() != nil {
		t.Fatal(iter.Err())
	}

	db.Get("arrows", a1.Id, &got)
	if !reflect.DeepEqual(newA1, got) {
		t.Fatal("didnt update")
	}
}

// Create a database and write to it, then close it, re-open with read-only and
// try to read what we wrote.
func TestReadOnly(t *testing.T) {
	db := OpenTestDB(t)
	defer db.Drop()

	a1 := Arrow{Id: "123", Length: 10, Sharpness: 0.97}
	db.Put("arrows", a1)

	db.Close()

	db2 := db.OpenAgain(SetReadOnly(true))
	defer db2.Drop()

	db2.DontGet("arrows", fmt.Sprint(rand.Intn(math.MaxInt64)))
	db2.DontGet("arrows2", fmt.Sprint(rand.Intn(math.MaxInt64)))

	var a2 Arrow
	db2.Get("arrows", a1.Id, &a2)
	if !reflect.DeepEqual(a1, a2) {
		t.Fatal("Get returns wrong data in read-only")
	}

	iter := db2.DB().Bucket("arrows").Iter()
	defer iter.Close()
	var a Arrow
	if !iter.Next(&a) {
		t.Fatal("Iter returns nothing in read-only")
	}
	if iter.Next(&a) {
		t.Fatal("Iter reads more than we wrote in read-only")
	}
	if iter.Err() != nil {
		t.Fatal(iter.Err())
	}
	if !reflect.DeepEqual(a, a2) {
		t.Fatal("Iter returns wrong data in read-only")
	}
}

type TestDB struct {
	t       *testing.T
	db      *DB
	closed  bool
	dir     string
	options []Option
	msg     string
	fail    func(...interface{})
}

func OpenTestDB(t *testing.T, options ...Option) *TestDB {
	tdb := &TestDB{
		t:    t,
		dir:  tempfile("bow-"),
		fail: t.Fatal,
	}
	tdb.Open(options...)
	return tdb
}

func (t *TestDB) OpenAgain(options ...Option) *TestDB {
	tdb := &TestDB{
		t:    t.t,
		dir:  t.dir,
		fail: t.fail,
	}
	tdb.Open(options...)
	return tdb
}

func (t *TestDB) Open(options ...Option) {
	var err error
	t.db, err = Open(t.dir, options...)
	if err != nil {
		t.fail(err)
	}
}

func (t *TestDB) Put(bucket string, v interface{}) {
	if err := t.db.Bucket(bucket).Put(v); err != nil {
		if t.msg != "" {
			err = fmt.Errorf("%s: %v", t.msg, err)
		}
		t.fail(err)
	}
}

func (t *TestDB) Get(bucket string, key, v interface{}) {
	if err := t.db.Bucket(bucket).Get(key, v); err != nil {
		if t.msg != "" {
			err = fmt.Errorf("%s: %v", t.msg, err)
		}
		t.fail(err)
	}
}

// DontGet fails if Get doesn't return ErrNotFound.
func (t *TestDB) DontGet(bucket string, key interface{}) {
	var a Arrow
	err := t.db.Bucket(bucket).Get(key, &a)
	if err != ErrNotFound {
		t.fail(err)
	}
}

func (t *TestDB) Fatal() *TestDB {
	tt := *t
	tt.fail = t.t.Fatal
	return &tt
}

func (t *TestDB) Error() *TestDB {
	tt := *t
	tt.fail = t.t.Error
	return &tt
}

func (t *TestDB) Msg(format string, a ...interface{}) *TestDB {
	tt := *t
	tt.msg = fmt.Sprintf(format, a...)
	return &tt
}

func (t *TestDB) DB() *DB {
	return t.db
}

func (t *TestDB) Close() {
	if t.closed {
		return
	}
	err := t.db.Close()
	if err != nil {
		t.fail(err)
	}
	t.closed = true
}

func (t *TestDB) Drop() {
	defer os.RemoveAll(t.dir)
	t.Close()
}

// tempfile returns a temporary file path.
func tempfile(prefix string) string {
	f, err := ioutil.TempFile("", prefix)
	if err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
	if err := os.Remove(f.Name()); err != nil {
		panic(err)
	}
	return f.Name()
}
