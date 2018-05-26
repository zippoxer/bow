# Bow [![GoDoc](https://godoc.org/github.com/zippoxer/bow?status.svg)](https://godoc.org/github.com/zippoxer/bow)

Bow is a minimal embedded database powered by Badger. 

The mission of Bow is to provide a simple, fast and reliable way to persist structured data for projects that don't need an external database server such as PostgreSQL or MongoDB.

Bow is powered by [BadgerDB](https://github.com/dgraph-io/badger), implementing buckets and serialization on top of it.

## Table of Contents

* [Why Badger and not Bolt?](#why-badger-and-not-bolt)
* [Getting Started](#getting-started)
  + [Installing](#installing)
  + [Opening a database](#opening-a-database)
  + [Defining a structure](#defining-a-structure)
    - [Randomly generated keys](#randomly-generated-keys)
  + [Persisting a structure](#persisting-a-structure)
  + [Retrieving a structure](#retrieving-a-structure)
  + [Iterating a bucket](#iterating-a-bucket)
    - [Prefix iteration](#prefix-iteration)
  + [Serialization](#serialization)
    - [MessagePack with `tinylib/msgp`](#messagepack-with-tinylibmsgp)
* [Upcoming](#upcoming)
  + [Key-only iteration](#key-only-iteration)
  + [Transactions](#transactions)
  + [Querying](#querying)
* [Performance](#performance)
* [Contributing](#contributing)

## Why Badger and not Bolt?
[Badger](https://github.com/dgraph-io/badger) is more actively maintained than [bbolt](https://github.com/coreos/bbolt), allows for key-only iteration and has some [very interesting performance characteristics](https://blog.dgraph.io/post/badger/).

## Getting Started

### Installing

```bash
go get -u github.com/zippoxer/bow
```

### Opening a database
```go
// Open database under directory "test".
db, err := bow.Open("test")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```

With options:

```go
db, err := bow.Open("test",
    bow.SetCodec(msgp.Codec{}),
    bow.SetBadgerOptions(badger.DefaultOptions}))
if err != nil {
    log.Fatal(err)
}
```

### Defining a structure

Each record in the database has an explicit or implicit unique key.

If a structure doesn't define a key or has a zero-value key, Bow stores it with a randomly generated key.

```go
type Page struct {
    Body     []byte
    Tags     []string
    Created  time.Time
}
```

Structures must define a key if they wish to manipulate it.

```go
type Page struct {
    Id      string `bow:"key"`
    Body     []byte
    Tags     []string
    Created  time.Time
}
```

Keys must be a string, a byte slice, any built-in integer or a type that implements [`codec.Marshaler`](https://godoc.org/github.com/zippoxer/bow/codec#Marshaler) and [`codec.Unmarshaler`](https://godoc.org/github.com/zippoxer/bow/codec#Unmarshaler).

#### Randomly generated keys

[`Id`](https://godoc.org/github.com/zippoxer/bow#Id) is a convenient placeholder for Bow's randomly generated keys.

```go
type Page struct {
    Id bow.Id // Annotating with `bow:"key"` isn't necessary.
    // ...
}
```

`Id.String()` returns a user-friendly representation of `Id`.

`ParseId(string)` parses the user-friendly representation into an `Id`.

`NewId()` generates a random Id. Only necessary when you need to know the inserted Id.

### Persisting a structure

`Put` persists a structure into the bucket. If a record with the same key already exists, then it will be updated.

```go
page1 := Page{
    Id:      bow.NewId(),
    Body:    []byte("<h1>Example Domain</h1>"),
    Tags:    []string{"example", "h1"},
    Created: time.Now(),
}
err := db.Bucket("pages").Put(page1)
if err != nil {
    log.Fatal(err)
}
```

### Retrieving a structure

`Get` retrieves a structure by key from a bucket, returning ErrNotFound if it doesn't exist.

```go
var page2 Page
err := db.Bucket("pages").Get(page1.Id, &page2)
if err != nil {
    log.Fatal(err)
}
```

### Iterating a bucket

```go
iter := db.Bucket("pages").Iter()
defer iter.Close()
var page Page
for iter.Next(&page) {
    log.Println(page.Id.String()) // User-friendly representation of bow.Id.
}
if iter.Err() != nil {
    log.Fatal(err)
}
```

#### Prefix iteration

Iterate over records whose key starts with a given prefix.

For example, let's define `Page` with URL as the key:

```go
type Page struct {
    URL  string `bow:"key"`
    Body []byte
}
```

Finally, let's iterate over HTTPS pages:

```go
iter := db.Bucket("pages").Prefix("https://")
var page Page
for iter.Next(&page) {
    log.Println(page.URL)
}
```

### Serialization

By default, Bow serializes structures with `encoding/json`. You can change that behaviour by passing a type that implements `codec.Codec` via the `bow.SetCodec` option. 

#### MessagePack with `tinylib/msgp`

msgp is a code generation tool and serialization library for [MessagePack](https://msgpack.org/), and it's nearly as fast as [protocol buffers](https://github.com/gogo/protobuf) and about an order of magnitude faster than encoding/json (see [benchmarks](https://github.com/alecthomas/go_serialization_benchmarks)). Bow provides a `codec.Codec` implementation for msgp under the `codec/msgp` package. Here's how to use it:

* Since msgp generates code to serialize structures, you must include the following directive in your Go file:

```go
//go:generate msgp
```

* Replace any use of `bow.Id` in your structures with `string`. Since `bow.Id` is a `string`, you can convert between the two without any cost.

* Import `github.com/zippoxer/bow/codec/msgp` and open a database with `msgp.Codec`:
```go
bow.Open("test", bow.SetCodec(msgp.Codec{}))
```

Read more about msgp and it's code generation settings at https://github.com/tinylib/msgp

## Upcoming

### Key-only iteration

Since Badger separates keys from values, key-only iteration should be orders of magnitude faster, at least in some cases, than it's equivalent with Bolt.

Bow's key-only iterator is a work in progress.

### Transactions

Cross-bucket transactions are a work in progress. See branch [tx](https://github.com/zippoxer/bow/tree/tx).

### Querying

Bow doesn't feature a querying mechanism yet. Instead, you must iterate records to query or filter them.

For example, let's say I want to perform the equivalent of

```SQL
SELECT * FROM pages WHERE url LIKE '%/home%'
```

in Bow, I could iterate the pages bucket and filter pages with URLs containing '/home':

```go
var matches []Page
iter := db.Bag("pages").Iter()
defer iter.Close()
var page Page
for iter.Next(&page) {
    if strings.Contains(strings.ToLower(page.URL), "/home") {
        matches = append(matches, page)
    }
}
return matches, iter.Err()
```

Meanwhile, you can try [Storm](https://github.com/asdine/storm) if you want convenient querying.

## Performance

Bow is nearly as fast as Badger, and in most cases faster than [Storm](https://github.com/asdine/storm). See [Go Database Benchmarks](https://github.com/zippoxer/go_database_bench).

## Contributing

I welcome any feedback and contribution.

My priorities right now are tests, documentation and polish.

Bow lacks decent tests and documentation for types, functions and methods. Most of Bow's code was written before I had any plan to release it, and I think it needs polish.
