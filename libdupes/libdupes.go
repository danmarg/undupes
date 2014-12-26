package libdupes

import (
	md5 "crypto/md5"
	"github.com/cenkalti/log"
	"io"
	"os"
	filepath "path/filepath"
	"sort"
)

type filesWithHashes struct {
	Unhashed string
	Hashes   map[[md5.Size]byte][]string
}

// Info contains information on duplicate file sets--specifically, the per-file size in bytes and the file paths.
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
		log.Warningln(err)
		return sum, err
	}
	defer f.Close()
	h := md5.New()
	_, err = io.Copy(h, f)
	if err != nil {
		log.Warningln(err)
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

// Dupes finds all duplicate files starting at the directory specified by "root". If specified, progressCb will be called to update the file processing progress.
func Dupes(root string, progressCb func(cur int, outof int)) ([]Info, error) {
	// Get files.
	// In order to enable the progress callback, we first list all the files (which should be relatively cheap) and then reiterate through the index to actually detect duplicates.
	pending := make(map[string]uint64)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		pending[path] = uint64(info.Size())
		return nil
	})
	files := make(map[uint64]*filesWithHashes)
	i := 0
	for path, size := range pending {
		if progressCb != nil {
			progressCb(i, len(pending))
		}
		i++
		hs, ok := files[size]
		if !ok {
			// If we've never seen another file of this size, we don't have to do the md5 sum.
			files[size] = &filesWithHashes{
				Unhashed: path,
				Hashes:   make(map[[md5.Size]byte][]string),
			}
			continue
		}
		// Add the hash of this file.
		sum, err := hash(path)
		if err != nil {
			log.Warningln(err)
			continue
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
				log.Warningln(err)
				continue
			}
			fs, ok := hs.Hashes[sum]
			if !ok {
				hs.Hashes[sum] = []string{hs.Unhashed}
			} else {
				hs.Hashes[sum] = append(fs, hs.Unhashed)
			}
			hs.Unhashed = ""
		}
	}
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
