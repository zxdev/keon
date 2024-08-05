package keon

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zxdev/xxhash"
)

/*
	KE:ON is a cookoo style hash table that distributes and rebalances keys
	across alternative index locations for membership testing. It is
	similar to a map[string]bool only faster and more memory efficient and
	offers a density compaction factor as well as save/load functions.

	key|key|key
	key|key|key
	...
	key|key|key

	Note: this is not mutex protected so it is not safe to read/write
	at the time same time, but it is safe for concurrent reads.

	kn := keon.NewSEON(uint64(count))
	kn.FailOnCOllision() // only when interested
	insert := kn.Insert()
	lookup := kn.Lookup()
	remove := kn.Remove()
	for ... {
		if !insert(key) {
			// handle error
		}
	}
*/

// KEON constants
const (
	width          = 3   // width of index bucket block [ key|key|key ]
	shuffleCycles  = 500 // max outer loop shuffle cycles
	shuffleHistory = 50  // max inner loop cyclic detection shuffle cycles (width * 17 appears ideal)
	name           = "keon"
)

// KEON is a set-only hash table structure
type KEON struct {
	path string // path to file
	//checksum   uint64   // checksum number for data integrity validation check; file i/o methods
	count, max uint64   // count of items, and max items
	depth      uint64   // depth for indexer
	key        []uint64 // key slice
}

/*
	keon package level functions
		NewKEON, Info, Load

*/

// NewKEON is the *KEON constructor.
func NewKEON(n uint64) *KEON {
	return new(KEON).sizer(n) // empty sized conainer
}

// Info will read and return *KEON file header information.
func Info(path string) (result struct {
	Checksum, Count, Max, depth uint64
	Ok                          bool
}) {

	f, err := os.Open(path)
	if err == nil {
		buf := bufio.NewReader(f)
		_, err = fmt.Fscanln(buf, &result.Checksum, &result.Count, &result.Max, &result.depth)
		f.Close()
	}

	// validate the keon is readable and the header is valid
	result.Ok = err == nil && result.Checksum > 0 && result.Max > 0

	return

}

// Load a *KEON from disk and the kn.valid validation status.
func Load(path string) (*KEON, bool) {

	kn := &KEON{path: path}
	kn.ext()

	f, err := os.Open(kn.path)
	if err != nil {
		return nil, false // bad file
	}
	defer f.Close()
	kn.path = path
	var valid uint64
	buf := bufio.NewReader(f)
	fmt.Fscanln(buf, &valid, &kn.count, &kn.max, &kn.depth)

	var k [8]byte
	var i uint64
	kn.sizer(0) // kn.max configured with load
	for {
		_, err = io.ReadFull(buf, k[:])
		if err != nil {
			// io.EOF or io.UnexpectedEOF
			return kn, valid == kn.validation()
		}
		kn.key[i] = binary.LittleEndian.Uint64(k[:])
		i++
	}

}

/*
	KEON utility and information methods
		sizer
		Len, Cap, Ratio, Ident

*/

// Density compaction scaling factor (default 40)
//
//	20 = 95.00% +10,000/20 adds 5.00% +500 pad
//	40 = 97.50% +10,000/40 adds 2.50% +250 pad
//	80 = 99.75% +10,000/80 adds 1.25% +125 pad
var Density uint64 = 40

// sizer configures KEON.key slice based on size requirement and density
func (kn *KEON) sizer(n uint64) *KEON {

	if n != 0 {
		kn.max = n
	}

	kn.depth = kn.max / width                           // calculate depth
	kn.depth += kn.depth / Density                      // add extra density space
	if kn.depth*width < kn.max || kn.depth%width != 0 { // ensure space requirements
		kn.depth++
	}

	kn.key = make([]uint64, kn.depth*width) // +width padding
	return kn
}

// validation generates a checksum number for data integrity validation
func (kn *KEON) validation() (checksum uint64) {
	for i := range kn.key {
		checksum = kn.key[i] ^ checksum // XOR
	}
	return checksum
}

// Len is number of current entries.
func (kn *KEON) Len() uint64 { return kn.count }

// Cap is max capacity of *KEON.
func (kn *KEON) Cap() uint64 { return kn.max }

// Ratio is fill ratio of *KEON.
func (kn *KEON) Ratio() uint64 {
	if kn.max == 0 {
		return 0
	}
	return kn.count * 100 / kn.max
}

/*
	KEON file i/o methods
		keon.Load
		kn.Create, kn.Save

*/

// Write *KEON to disk at path.
func (kn *KEON) Write(path string) error {
	kn.path = path
	kn.ext()
	return kn.Save()
}

// ext validates the file has a .keon extension
func (kn *KEON) ext() {
	if len(kn.path) == 0 {
		kn.path = name
	}
	if !strings.HasSuffix(kn.path, ".keon") {
		kn.path += ".keon"
	}
}

// Save *KEON to disk at prior Load/Write path
func (kn *KEON) Save() error {

	kn.ext()

	f, err := os.Create(kn.path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := bufio.NewWriter(f)
	fmt.Fprintln(buf, kn.validation(), kn.count, kn.max, kn.depth)

	var b [8]byte
	for i := uint64(0); i < uint64(len(kn.key)); i++ {
		binary.LittleEndian.PutUint64(b[:], kn.key[i])
		buf.Write(b[:])
	}

	buf.Flush()
	return f.Sync()
}

/*
	KEON primary management methods
		Lookup, Remove, Insert

*/

// the last item in the indexer is the key hash, so keyIndex must
// always be equal to len(indexer)-1; hence the const declaration
const keyIndex = 3 // 0,1,2

type indexer [keyIndex + 1]uint64 // 3

// calculates target index locations
func (idx *indexer) calculate(size uint64) {
	idx[0] = width * (idx[keyIndex] % size)
	idx[1] = width * ((idx[keyIndex] ^ 11400714785074694791) % size) // prime1 11400714785074694791
	idx[2] = width * ((idx[keyIndex] ^ 9650029242287828579) % size)  // prime4 9650029242287828579
	// idx[3] holds hash of key
}

// Lookup key in *KEON.
func (kn *KEON) Lookup() func(key []byte) bool {

	var idx indexer
	var n, i, j uint64

	return func(key []byte) bool {

		idx[keyIndex] = xxhash.Sum(key)
		idx.calculate(kn.depth)

		for i = 0; i < keyIndex; i++ {
			for j = 0; j < width; j++ {
				n = idx[i] + j
				if kn.key[n] == idx[keyIndex] {
					return true
				}
			}
		}

		return false
	}
}

// Remove key from *KEON.
func (kn *KEON) Remove() func(key []byte) bool {

	var idx indexer
	var n, i, j uint64

	return func(key []byte) bool {

		idx[keyIndex] = xxhash.Sum((key))
		idx.calculate(kn.depth)

		for i = 0; i < keyIndex; i++ {
			for j = 0; j < width; j++ {
				n = idx[i] + j
				if kn.key[n] == idx[keyIndex] {
					copy(kn.key[n:n+width-j], kn.key[n+1:n+1+width-j]) // shift segment
					kn.key[idx[i]+width-1] = 0                         // wipe tail
					kn.count--
				}
			}
		}

		return false
	}
}

// Insert into *KEON.
//
//	Ok flag on insert success
//	Exist flag when already present (or collision)
//	NoSpace flag with at capacity or shuffler failure
func (kn *KEON) Insert() func(key []byte) struct{ Ok, Exist, NoSpace bool } {

	var idx indexer
	var n, i, j uint64
	var ix, jx uint64
	var empty bool

	var node [2]uint64
	var cyclic map[[2]uint64]uint8

	return func(key []byte) (result struct{ Ok, Exist, NoSpace bool }) {

		if kn.count == kn.max {
			result.NoSpace = true
			return
		}

		idx[keyIndex] = xxhash.Sum(key)
		idx.calculate(kn.depth)
		empty = false

		// verify not already present in any target index location
		// and record the next insertion point while checking
		for i = 0; i < keyIndex; i++ {
			for j = 0; j < width; j++ {
				n = idx[i] + j
				if kn.key[n] == idx[keyIndex] {
					result.Exist = true
					return
				}
				if kn.key[n] == 0 && !empty {
					empty = true
					ix, jx = i, j
				}
			}
		}

		// insert the new key at ix,jx target
		if empty {
			kn.key[idx[ix]+jx] = idx[keyIndex]
			kn.count++
			result.Ok = true
			return
		}

		// shuffle and displace a key to allow for current key insertion using an
		// outer loop composed of many short inner shuffles that succeed or fail quickly
		// to cycle over many alternate short path swaps that abort on cyclic movements
		var random [8]byte
		for jx = 0; jx < shuffleCycles; jx++ { // 500 cycles of 50 smaller swap tracks
			cyclic = make(map[[2]uint64]uint8, shuffleHistory) // cyclic movement tracker

			for {
				rand.Read(random[:])
				ix = idx[binary.LittleEndian.Uint64(random[:8])%width] // select random altenate index to use
				n = ix + uint64(random[7]%width)                       // select random key to displace and swap
				node = [2]uint64{ix, idx[keyIndex]}                    // cyclic node generation; index and key
				cyclic[node]++                                         // cyclic recurrent node movement tracking
				if cyclic[node] > width || len(cyclic) == shuffleHistory {
					break // reset cyclic path tracker and jump tracks by picking a new random index
					// and key to displace as this gives us about ~2x faster performance boost by
					// locating an open slot faster for some reason
				}

				kn.key[n], idx[keyIndex] = idx[keyIndex], kn.key[n] // swap keys to displace the key
				idx.calculate(kn.depth)                             // generate index set for displaced key

				for i = 0; i < keyIndex; i++ { // attempt to insert displaced key in alternate location
					if idx[i] != ix { // avoid the common index between key and displaced key
						for j = 0; j < width; j++ {
							n = idx[i] + j
							if kn.key[n] == 0 { // a new location for displaced key
								kn.key[n] = idx[keyIndex]
								kn.count++
								result.Ok = true
								return
							}
						}
					}
				}

			}
		}

		// ran out of key shuffle options
		result.NoSpace = true
		return
	}
}
