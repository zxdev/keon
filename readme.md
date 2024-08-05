# KEON

This is a membership key set package for use when detecting if an item key belongs to a reference set. It is akin to map[uint64]bool, but uses about 1/6th of the RAM; requires a known size to build the binary. 

This follows a MRSW philosophy, so is always thread safe to read or write, but isn't safe to read and write at the same time. It does not have a hard locking mechanism to prevent inappropriate use, such as a mutex or atomics, so a caller must build or load first, then do lookups. In other words, multiple go routines can read from this without issue, however anything that modifies the entries must be a one process job, hence an MRSW that must be adheared to to avoid data-race conditions. 

A reference set may be saved or loaded, and a magic number is generated and compared to prove data integrity. Items may be removed from the set, however the desired container size is static once it has been created and can not be resized dynamically. If it must be resized, it but be rebuild as there is currently no key iterator to facilitate a resize operation since this is not currently a use case requirement.

This architecture is a three bucket wide type of cookoo hash table with some additional performance enhancements, arraged logically like in a row/bucket pattern:

  key|key|key
  key|key|key
  ...
  key|key|key

The compaction ```density``` is currently hard coded to 97.5% (40) in the current iteration as this provides the most reasonable trade off between memory, insert performance, and table utilization. The key shuffler and cyclic movement detector will shuffle a randomly selected keys within a scope of smaller shuffle tracks with cyclic monitored movements and this has proven to be adequate up to 99.75% (80) table density on large tables of 100MM. Small tables under 25,000 could be naturally arranged into a minimal-perfect-hash arrangement at high density. Modifying the ```shuffleCycles``` (default 500) manages the external track loop, so higher track trials means higher potential density at longer insert times near capacity. Modifying the ```shuffleHistory``` (default 50) manages the cyclic movement detection within an individual track trial. With the current bucket width (default 3), quickly resetting on a new track trial when not successful on the inner loop has proven to be worth about ~2x performance gain.

The ```density math compaction factor``` is effectively adding a percentage of empty padding space while only using integer numberics to calculate it. For example, table_size / number_of_buckets is equal to the column_depth, and to prevent the key shuffler from operating as a minimal-perfect-hash tabler with progressively longer insertion times as capacity is reached, a compaction density factor allows for additional bucket padding to maintain operational performance within spec.

  20 = 95.00% +depth/20 10,000 adds 500
  40 = 97.50% +depth/40 10,000 adds 250
  80 = 99.75% +depth/80 10,000 adds 125

Insert reports three conditions.
* Ok, insert was successful.
* Exist, key already exists (or possible hash collision)
* NoSpace, when at max capacity or insert shuffer failure

With 1e6 records trial, the following code performance was observed with a 2012 mac mini with a 2.3 GHz Quad-Core Intel Core i7 and 16GB ram.

* keon_test.go:24: 370.543827ms for 1e6 insert, about 2.7 million/sec
* keon_test.go:35: 111.005397ms for 1e6 lookup, about 9 million/sec

Scaling factor for go routine readers has been observed to approximately 1.5x per CPU, so in theory a 4 core machine could support reads of approximately 54 million/sec with 6 go routine readers accessing a KEON.

```golang

  size := uint64(1000000)
  kn := keon.NewKEON(size)
  kn.FailOnCollision()
  insert := kn.Insert()
  lookup := kn.Lookup()

  t0 := time.Now()
  for i := uint64(0); i < size; i++ {
    if !insert([]byte{byte(i % 255), byte(i % 26), byte(i % 235), byte(i % 254), byte(i % 249), byte(i % 197), byte(i % 17), byte(i % 99)}) {
      t.Log("insert failure", i)
      t.FailNow()
    }
  }
  fmt.Log(time.Since(t0))

  t0 = time.Now()
  for i := uint64(0); i < size; i++ {
    if !lookup([]byte{byte(i % 255), byte(i % 26), byte(i % 235), byte(i % 254), byte(i % 249), byte(i % 197), byte(i % 17), byte(i % 99)}) {
      t.Log("lookup failure", i)
      t.FailNow()
    }
  }
  fmt.Log(time.Since(t0))
  
```
