package libdupes

import (
	md5 "crypto/md5"
	"github.com/cenkalti/log"
	"io"
	"os"
	filepath "path/filepath"
	"sort"
)

// In our first pass, only hash the first initialBlocksize bytes of a file.
const initialBlocksize = 4096

type filesWithHashes struct {
	Unhashed        string
	FirstPassHashes map[[md5.Size]byte]string
	FullHashes      map[[md5.Size]byte][]string
}

// Info contains information on duplicate file sets--specifically, the per-file size in bytes and the file paths.
type Info struct {
	Size  int64
	Names []string
}

type bySize []Info

func (a bySize) Len() int      { return len(a) }
func (a bySize) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a bySize) Less(i, j int) bool {
	return a[i].Size*int64(len(a[i].Names)) < a[j].Size*int64(len(a[j].Names))
}

// Hash a file at path. If blocksize is >0, only hash up to the first blocksize bytes.
func hash(path string, blocksize int64) ([md5.Size]byte, error) {
	var sum [md5.Size]byte
	f, err := os.Open(path)
	if err != nil {
		log.Warningln(err)
		return sum, err
	}
	defer f.Close()
	h := md5.New()
	if blocksize > 0 {
		_, err = io.CopyN(h, f, blocksize)
	} else {
		_, err = io.Copy(h, f)
	}
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

func min(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

// Dupes finds all duplicate files starting at the directory specified by "root". If specified, progressCb will be called to update the file processing progress.
func Dupes(roots []string, progressCb func(cur int, outof int)) ([]Info, error) {
	// Get files.
	// In order to enable the progress callback, we first list all the files (which should be relatively cheap) and then reiterate through the index to actually detect duplicates.
	pending := make(map[string]int64)
        for _, root := range roots {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		pending[path] = int64(info.Size())
		return nil
	})
      }
	files := make(map[int64]*filesWithHashes)
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
				Unhashed:        path,
				FirstPassHashes: make(map[[md5.Size]byte]string),
				FullHashes:      make(map[[md5.Size]byte][]string),
			}
			continue
		}
		// If there's an unhashed file, we have to compute the first-pass hashes.
		if hs.Unhashed != "" {
			// This should never ever happen, so we don't handle errors properly.
			if len(hs.FirstPassHashes) > 0 || len(hs.FullHashes) > 0 {
				panic("logic error!")
			}
			// First-pass hash of the unhashed.
			sum, err := hash(hs.Unhashed, min(initialBlocksize, size))
			if err != nil {
				log.Warningln(err)
				continue
			}
			hs.FirstPassHashes[sum] = hs.Unhashed
			hs.Unhashed = ""
		}
		// Now we compute the first-pass hash of the current file.
		sum, err := hash(path, min(initialBlocksize, size))
		if err != nil {
			log.Warningln(err)
			continue
		}
		collision, ok := hs.FirstPassHashes[sum]
		if ok {
			// Second-pass hashes required.
			if collision != "" {
				// Also have to do a second-pass hash of the previous.
				hs.FirstPassHashes[sum] = ""
				sum, err := hash(collision, -1)
				if err != nil {
					log.Warningln(err)
					continue
				}
				fs, _ := hs.FullHashes[sum]
				hs.FullHashes[sum] = append(fs, collision)
			}
			// And of the current file.
			sum, err := hash(path, -1)
			if err != nil {
				log.Warningln(err)
				continue
			}
			fs, _ := hs.FullHashes[sum]
			hs.FullHashes[sum] = append(fs, path)

		} else {
			hs.FirstPassHashes[sum] = path
		}
	}
	dupes := []Info{}
	for size, hs := range files {
		for _, files := range hs.FullHashes {
			if len(files) > 1 {
				dupes = append(dupes, Info{Size: size, Names: files})
			}
		}
	}

	sort.Sort(sort.Reverse(bySize(dupes)))
	return dupes, nil
}
