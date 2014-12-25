package main

import (
	"bufio"
	"fmt"
	"github.com/codegangsta/cli"
	dupes "github.com/danmarg/undupes/libdupes"
	"github.com/dustin/go-humanize"
	"log"
	"os"
	"strconv"
	"strings"
)

func main() {
	app := cli.NewApp()
	app.Name = "undupes"
	app.Usage = "manage duplicate files"
	app.Author = "Daniel Margolis"
	app.Email = "dan@af0.net"
	app.Version = "0.1"
	app.Commands = []cli.Command{
		{
			Name:      "interactive",
			ShortName: "i",
			Usage:     "interactive mode",
			Action: func(c *cli.Context) {
				runInteractively()
			},
		},
	}
	app.Run(os.Args)
}

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
func runInteractively() {
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
	fmt.Printf("Indexing...")
	dupes, err := dupes.Dupes(root)
	if err != nil {
		log.Fatal(err)
	}
	fcount := 0
	tsize := uint64(0)
	for _, i := range dupes {
		fcount += len(i.Names)
		tsize += i.Size * uint64(len(i.Names))
	}

	fmt.Printf("Found %d sets of duplicate files\n", len(dupes))
	fmt.Printf("Total file count: %d\n", fcount)
	fmt.Printf("Total size used: %s\n", humanize.Bytes(tsize))

	fmt.Printf("\nReviewing results:\nFor each duplicate fileset, select 'f' to delete all but the first file, 'a' to keep all files, or 'n' (e.g. 2) to delete all except the second file.\n")

	for i, dupe := range dupes {
		keep := -1
		names := ""
		for j, n := range dupe.Names {
			names += fmt.Sprintf("%d: %s\n", j+1, n)
		}
		_, err := getInput(fmt.Sprintf("\n%d of %d  %s:\n%s\nKeep [F]irst/[a]ll/[n]th? ", i+1, len(dupes), humanize.Bytes(dupe.Size*uint64(len(dupe.Names)-1)), names), func(v string) bool {
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
