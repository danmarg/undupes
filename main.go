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

type filesWithHashes struct {
	Unhashed string
	Hashes   map[[md5.Size]byte][]string
}

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
	files := make(map[uint64]*filesWithHashes)
	ps := []rune{'|', '/', '-', '\\', '|', '/', '-', '\\'}
	i := 0
	fmt.Printf("Indexing %s \n \r", root)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		i++
		fmt.Printf("%c \r", ps[(int(i/30))%len(ps)])
		hs, ok := files[uint64(info.Size())]
		if !ok {
			// If we've never seen another file of this size, we don't have to do the md5 sum.
			files[uint64(info.Size())] = &filesWithHashes{
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
	dupes := []info{}
	fcount := 0
	tsize := uint64(0)
	for size, hs := range files {
		for _, files := range hs.Hashes {
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

	fmt.Printf("\nReviewing results:\nFor each duplicate fileset, select 'f' to delete all but the first file, 'a' to keep all files, or 'n' (e.g. 2) to delete all except the second file.\n")

	for _, dupe := range dupes {
		keep := -1
		names := ""
		for i, n := range dupe.Names {
			names += fmt.Sprintf("%d: %s\n", i+1, n)
		}
		_, err := getInput(fmt.Sprintf("%s:\n%s\nKeep [F]irst/[a]ll/[n]th? ", humanize.Bytes(dupe.Size*uint64(len(dupe.Names)-1)), names), func(v string) bool {
			switch strings.ToLower(v) {
			case "f":
				keep = 0
				return true
			case "a":
				return true
			default:
				k, err := strconv.ParseInt(v, 10, 32)
				if err != nil {
					return false
				}
				if k < 1 || int(k) > len(dupe.Names) {
					return false
				}
				keep = int(k) - 1
				return true
			}
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
