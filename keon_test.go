package keon_test

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zxdev/keon"
)

func TestKEON(t *testing.T) {

	size := uint64(1000000)

	kn := keon.NewKEON(size)
	insert := kn.Insert()
	lookup := kn.Lookup()

	t0 := time.Now()
	for i := uint64(0); i < size; i++ {
		if !insert([]byte{byte(i % 255), byte(i % 26), byte(i % 235), byte(i % 254), byte(i % 249), byte(i % 197), byte(i % 17), byte(i % 99)}).Ok {
			t.Log("insert failure", i)
			t.FailNow()
		}
	}
	t.Log("insert", time.Since(t0))

	t0 = time.Now()
	for i := uint64(0); i < size; i++ {
		if !lookup([]byte{byte(i % 255), byte(i % 26), byte(i % 235), byte(i % 254), byte(i % 249), byte(i % 197), byte(i % 17), byte(i % 99)}) {
			t.Log("lookup failure", i)
			t.FailNow()
		}
	}
	t.Log("lookup", time.Since(t0))

	t0 = time.Now()
	t.Log("magic", time.Since(t0))

	t.Log("stats", kn.Len(), kn.Cap(), kn.Ratio())

}

func TestInfo(t *testing.T) {

	size := uint64(1000000)

	kn := keon.NewKEON(size)
	insert := kn.Insert()
	lookup := kn.Lookup()

	t0 := time.Now()
	for i := uint64(0); i < size; i++ {
		if !insert([]byte{byte(i % 255), byte(i % 26), byte(i % 235), byte(i % 254), byte(i % 249), byte(i % 197), byte(i % 17), byte(i % 99)}).Ok {
			t.Log("insert failure", i)
			t.FailNow()
		}
	}
	t.Log("insert", time.Since(t0))

	t0 = time.Now()
	for i := uint64(0); i < size; i++ {
		if !lookup([]byte{byte(i % 255), byte(i % 26), byte(i % 235), byte(i % 254), byte(i % 249), byte(i % 197), byte(i % 17), byte(i % 99)}) {
			t.Log("lookup failure", i)
			t.FailNow()
		}
	}
	t.Log("lookup", time.Since(t0))

	t.Log("stats", kn.Len(), kn.Cap(), kn.Ratio())

	kn.Write("keon")
	kn = nil

	info := keon.Info("keon")
	t.Log(info.Checksum, info.Count, info.Max, info.Ok)
	os.Remove("keon")

}

var fileDB = "keon.DB"

func TestONE(t *testing.T) {

	file := "ddump1e6"
	path := filepath.Join(os.Getenv("HOME"), "Development", "_data", file)
	show := false

	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	var count uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	t.Log(count)
	f.Seek(0, 0)

	kn := keon.NewKEON(uint64(count))
	insert := kn.Insert()
	lookup := kn.Lookup()

	count = 0
	t0 := time.Now()
	scanner = bufio.NewScanner(f)
	for scanner.Scan() {
		if !insert(scanner.Bytes()).Ok {
			t.Log("keon: lookup insert", count, scanner.Text()) //, xxHash(scanner.Bytes()))
			t.Fail()
		}
		count++
	}
	t.Log(count, time.Since(t0))

	f.Seek(0, 0)
	scanner = bufio.NewScanner(f)
	count = 0
	t0 = time.Now()
	for scanner.Scan() {
		if !lookup(scanner.Bytes()) {
			t.Log("keon: lookup failure", count, scanner.Text())
			t.Fail()
		}
		count++
	}
	t.Log(count, time.Since(t0))

	if show {
		dump := kn.Dump()
		for i := 0; i < len(dump); i += 3 {
			row := []uint64{}
			for j := 0; j < 3; j++ {
				row = append(row, dump[i+j])
			}
			t.Log(i, row)
		}
	}

	t0 = time.Now()
	kn.Write(fileDB)
	t.Log("keon: save", time.Since(t0))
}

func TestTWO(t *testing.T) {

	file := "ddump1e6"
	path := filepath.Join(os.Getenv("HOME"), "Development", "_data", file)

	t0 := time.Now()
	kn, ok := keon.Load(fileDB)
	if !ok {
		panic("no " + fileDB)
	}
	defer os.Remove(fileDB)
	t.Log("keon: load", time.Since(t0))
	lookup := kn.Lookup()

	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	t0 = time.Now()
	for scanner.Scan() {
		if !lookup(scanner.Bytes()) {
			t.Log("keon: lookup fail", count, scanner.Text())
			t.Fail()
		}
		count++
	}
	t.Log("keon: summary", count, time.Since(t0))

}
