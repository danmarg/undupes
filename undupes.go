package main

import (
	md5 "crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	filepath "path/filepath"
	"sort"
)

type filesWithHashes struct {
	Unhashed string
	Hashes   map[[md5.Size]byte][]string
}

type Info struct {
	Size  uint64
	Names []string
}

type bySize []Info

func (a bySize) Len() int      { return len(a) }
func (a bySize) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a bySize) Less(i, j int) bool {
	return a[i].Size*uint64(len(a[i].Names)) < a[j].Size*uint64(len(a[j].Names))
}

func hash(path string) ([md5.Size]byte, error) {
	var sum [md5.Size]byte
	f, err := os.Open(path)
	if err != nil {
		log.Print(err)
		return sum, err
	}
	defer f.Close()
	h := md5.New()
	_, err = io.Copy(h, f)
	if err != nil {
		log.Print(err)
		return sum, err
	}
	s := h.Sum(nil)
	if len(s) != len(sum) {
		panic("Unexpected checksum length")
	}
	for i, v := range s {
		sum[i] = v
	}
	return sum, nil
}

func Dupes(root string) ([]Info, error) {
	// Get files.
	files := make(map[uint64]*filesWithHashes)
	ps := []rune{'|', '/', '-', '\\', '|', '/', '-', '\\'}
	i := 0
	fmt.Printf("Indexing %s \n \r", root)
	filepath.Walk(root, func(path string, Info os.FileInfo, err error) error {
		if Info.IsDir() {
			return nil
		}
		i++
		fmt.Printf("%c \r", ps[(int(i/30))%len(ps)])
		hs, ok := files[uint64(Info.Size())]
		if !ok {
			// If we've never seen another file of this size, we don't have to do the md5 sum.
			files[uint64(Info.Size())] = &filesWithHashes{
				Unhashed: path,
				Hashes:   make(map[[md5.Size]byte][]string),
			}
			return nil
		}
		// Add the hash of this file.
		sum, err := hash(path)
		if err != nil {
			return err
		}
		fs, ok := hs.Hashes[sum]
		if !ok {
			hs.Hashes[sum] = []string{path}
		} else {
			hs.Hashes[sum] = append(fs, path)
		}
		// And check if there is also an unhashed file.
		if hs.Unhashed != "" {
			sum, err := hash(hs.Unhashed)
			if err != nil {
				return err
			}
			fs, ok := hs.Hashes[sum]
			if !ok {
				hs.Hashes[sum] = []string{hs.Unhashed}
			} else {
				hs.Hashes[sum] = append(fs, hs.Unhashed)
			}
			hs.Unhashed = ""
		}
		return nil
	})
	dupes := []Info{}
	for size, hs := range files {
		for _, files := range hs.Hashes {
			if len(files) > 1 {
				dupes = append(dupes, Info{Size: size, Names: files})
			}
		}
	}

	sort.Sort(sort.Reverse(bySize(dupes)))
	return dupes, nil
}
