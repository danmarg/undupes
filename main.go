package main

import (
	"bufio"
	md5 "crypto/md5"
	"fmt"
	"github.com/dustin/go-humanize"
	"io"
	"log"
	"os"
	filepath "path/filepath"
	"sort"
	"strconv"
	"strings"
)

type info struct {
	Size  uint64
	Names []string
}

type bySize []info

func (a bySize) Len() int           { return len(a) }
func (a bySize) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a bySize) Less(i, j int) bool { return a[i].Size < a[j].Size }

func getInput(prompt string, validator func(string) bool) (string, error) {
	// Reader to read from user input.
	reader := bufio.NewReader(os.Stdin)
	var (
		val string
		err error
	)
	for {
		fmt.Printf(prompt)
		val, err = reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		val = strings.TrimSpace(val)
		if validator(val) {
			break
		}
	}
	return val, nil
}

func main() {
	// Get parent dir.
	root, err := getInput("Enter parent directory to scan for duplicates in: ", func(f string) bool {
		i, err := os.Stat(f)
		if err != nil {
			log.Print(err)
			return false
		}
		return i.IsDir()
	})
	if err != nil {
		log.Fatal(err)
	}
	if !os.IsPathSeparator(root[len(root)-1]) {
		root = fmt.Sprintf("%s%c", root, os.PathSeparator)
	}
	// Get files.
	files := make(map[uint64]map[[md5.Size]byte][]string) // Size -> ( hash -> [name] )
	ps := []rune{'|', '/', '-', '\\', '|', '/', '-', '\\'}
	i := 0
	fmt.Printf("Indexing %s \n \r", root)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		i++
		fmt.Printf("%c \r", ps[(i%100)%len(ps)])
		hs, ok := files[uint64(info.Size())]
		if !ok {
			hs = make(map[[md5.Size]byte][]string)
			files[uint64(info.Size())] = hs
		}
		f, err := os.Open(path)
		if err != nil {
			log.Print(err)
			return err
		}
		defer f.Close()
		h := md5.New()
		_, err = io.Copy(h, f)
		if err != nil {
			log.Print(err)
			return err
		}
		var sum [md5.Size]byte
		s := h.Sum(nil)
		if len(s) != len(sum) {
			panic("Unexpected checksum length")
		}
		for i, v := range s {
			sum[i] = v
		}
		fs, ok := hs[sum]
		if !ok {
			hs[sum] = []string{path}
		} else {
			hs[sum] = append(fs, path)
		}

		return nil
	})
	dupes := []info{}
	fcount := 0
	tsize := uint64(0)
	for size, hashes := range files {
		for _, files := range hashes {
			if len(files) > 1 {
				dupes = append(dupes, info{Size: size, Names: files})
				fcount += len(files)
				tsize += size
			}
		}
	}
	fmt.Printf("Found %d sets of duplicate files\n", len(dupes))
	fmt.Printf("Total file count: %d\n", fcount)
	fmt.Printf("Total size used: %d\n", tsize)

	sort.Sort(sort.Reverse(bySize(dupes)))

	fmt.Printf("\nReviewing results: for each duplicate fileset, select 'f' to delete all but the first file, 'a' to keep all files, or 'n' followed by a file number (e.g. n2) to delete all except the second file.\n")

	for _, dupe := range dupes {
		keep := -1
		_, err := getInput(fmt.Sprintf("%s: %v\nKeep [F]irst/[a]ll/[n]th? ", humanize.Bytes(dupe.Size*uint64(len(dupe.Names)-1)), dupe.Names), func(v string) bool {
			switch strings.ToLower(v) {
			case "f":
				keep = 0
				return true
			case "a":
				return true
			default:
				if strings.HasPrefix(strings.ToLower(v), "n") {
					k, err := strconv.ParseInt(v[1:], 10, 32)
					if err != nil {
						return false
					}
					if k < 1 || int(k) > len(dupe.Names) {
						return false
					}
					keep = int(k) - 1
					return true
				} else {
					return false
				}
			}
			return false
		})
		if err != nil {
			log.Fatal(err)
		}
		if keep >= 0 {
			for i, n := range dupe.Names {
				if i != keep {
					if err := os.Remove(n); err != nil {
						log.Printf("Error deleting %n: %v", n, err)
					}
				}
			}
		}
	}
}
