package main

import (
	"bufio"
	"fmt"
	"github.com/cenkalti/log"
	"github.com/codegangsta/cli"
	dupes "github.com/danmarg/undupes/libdupes"
	"github.com/dustin/go-humanize"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Horrible hack to convert -v to log Levels.
var orderedLevels []log.Level = []log.Level{log.DEBUG, log.INFO, log.NOTICE, log.WARNING, log.ERROR, log.CRITICAL}

func setLogLevel(l int) error {
	if l >= 0 && l < len(orderedLevels) {
		log.SetLevel(orderedLevels[l])
		return nil
	}
	return fmt.Errorf("invalid log level specified")
}

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
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "v",
					Usage: "log level",
					Value: 3, // Don't print as much in interactive mode.
				},
			},
			Action: func(c *cli.Context) {
				if err := setLogLevel(c.Int("v")); err != nil {
					fmt.Println(err)
					return
				}
				if err := runInteractive(c.Bool("dry_run")); err != nil {
					fmt.Println(err)
				}
			},
		},
		{
			Name:      "print",
			ShortName: "p",
			Usage:     "print duplicates",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "directory, d",
					Usage: "directory in which to find duplicates (required)",
				},
				cli.StringFlag{
					Name:  "output, o",
					Usage: "output file",
				},
			},
			Action: func(c *cli.Context) {
				if err := runPrint(c.String("directory"), c.String("output")); err != nil {
					fmt.Println(err)
				}
			},
		},
		{
			Name:      "auto",
			ShortName: "a",
			Usage:     "automatically resolve duplicates",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "directory, d",
					Usage: "directory in which to find duplicates (required)",
				},
				cli.StringFlag{
					Name:  "prefer, p",
					Usage: "prefer to keep files matching this pattern",
				},
				cli.StringFlag{
					Name:  "over, o",
					Usage: "used with --prefer; will restrict preferred files to those where the duplicate matches --over",
				},
				cli.BoolFlag{
					Name:  "invert, i",
					Usage: "invert matching logic; preferred files with be prioritized for deletion rather than retention",
				},
				cli.BoolFlag{
					Name:  "dry_run",
					Usage: "simulate (log but don't delete files)",
				},
				cli.IntFlag{
					Name:  "v",
					Usage: "log level",
					Value: 1,
				},
			},
			Action: func(c *cli.Context) {
				// Check for required arguments.
				if c.String("directory") == "" {
					fmt.Println("--directory is required")
					return
				}
				if c.String("prefer") == "" {
					fmt.Println("--prefer is required")
					return
				}
				if err := setLogLevel(c.Int("v")); err != nil {
					fmt.Println(err)
					return
				}
				// Compile regexps.
				var prefer, over *regexp.Regexp
				var err error
				if prefer, err = regexp.Compile(c.String("prefer")); err != nil {
					fmt.Println("invalid regexp specified for --prefer")
					return
				}
				if c.String("over") != "" {
					if over, err = regexp.Compile(c.String("over")); err != nil {
						fmt.Println("invalid regexp specified for --over")
						return
					}
				}
				// Do deduplication.
				if err = runAutomatic(c.Bool("dry_run"), c.String("directory"), prefer, over, c.Bool("invert")); err != nil {
					fmt.Println(err)
				}
			},
		},
	}
	app.RunAndExitOnError()
}

func remove(dryRun bool, file string) {
	if dryRun {
		log.Noticef("DRY RUN: delete %s", file)
	} else {
		if err := os.Remove(file); err != nil {
			log.Warningf("error deleting %n: %v", file, err)
		} else {
			log.Noticef("deleted %n", file)
		}
	}

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

func getDupesAndPrintSummary(root string) ([]dupes.Info, error) {
	if !os.IsPathSeparator(root[len(root)-1]) {
		root = fmt.Sprintf("%s%c", root, os.PathSeparator)
	}
	fmt.Printf("Indexing...")
	dupes, err := dupes.Dupes(root)
	if err != nil {
		return dupes, err
	}
	fcount := 0
	tsize := uint64(0)
	for _, i := range dupes {
		fcount += len(i.Names) - 1
		tsize += i.Size * uint64(len(i.Names)-1)
	}

	fmt.Printf("\rFound %d sets of duplicate files\n", len(dupes))
	fmt.Printf("Total file count: %d\n", fcount)
	fmt.Printf("Total size used: %s\n", humanize.Bytes(tsize))
	return dupes, err
}

func runInteractive(dryRun bool) error {
	// Get parent dir.
	root, err := getInput("Enter parent directory to scan for duplicates in: ", func(f string) bool {
		i, err := os.Stat(f)
		if err != nil {
			return false
		}
		return i.IsDir()
	})
	if err != nil {
		return err
	}
	dupes, err := getDupesAndPrintSummary(root)
	if err != nil {
		return err
	}

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
			log.Fatalln(err)
		}
		if keep >= 0 {
			for i, n := range dupe.Names {
				if i != keep {
					remove(dryRun, n)
				}
			}
		}
	}
	return nil
}

func runPrint(root string, output string) error {
	dupes, err := getDupesAndPrintSummary(root)
	if err != nil {
		return err
	}

	var f *os.File
	if output != "" {
		if f, err = os.Create(output); err != nil {
			return err
		}
	}
	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				log.Warningln(err)
			}
		}
	}()
	for _, dupe := range dupes {
		l := fmt.Sprintf("%s * %d => %s\n", humanize.Bytes(dupe.Size), len(dupe.Names), strings.Join(dupe.Names, ", "))
		if f != nil {
			f.Write([]byte(l))
		} else {
			fmt.Printf(l)
		}
	}
	return nil
}

func runAutomatic(dryRun bool, root string, prefer *regexp.Regexp, over *regexp.Regexp, invert bool) error {
	dupes, err := getDupesAndPrintSummary(root)
	if err != nil {
		return err
	}

	for _, dupe := range dupes {
		p := make(map[string]struct{})
		o := make(map[string]struct{})
		for _, n := range dupe.Names {
			nb := []byte(n)
			pm := prefer.Match(nb)
			om := false
			if over != nil {
				om = over.Match(nb)
			}
			if pm && om {
				log.Warningf("both --prefer and --over matched %s", n)
			}
			if pm {
				p[n] = struct{}{}
			} else if om {
				o[n] = struct{}{}
			}
		}
		if len(p) > 0 { // If we found a preferred match.
			// Generate debug line.
			dbg := fmt.Sprintf("processing %s\n\tprefer: ", strings.Join(dupe.Names, ", "))
			for k := range p {
				dbg += k + ", "
			}
			if len(o) > 0 {
				dbg += "\n\tover: "
				for k := range o {
					dbg += k + ", "
				}
			}
			log.Debugf("%s", dbg)
			// Logic is here.
			if over != nil {
				if len(o) > 0 {
					// If prefer and over are both specified, and both match, remove the non-preferred matches.
					if invert {
						for n := range p {
							remove(dryRun, n)
						}
					} else {
						for n := range o {
							remove(dryRun, n)
						}
					}
				}
			} else {
				// If over is not specified, keep only the preferred, but (for the case of --invert) only when preferred is not everything.
				if len(p) < len(dupe.Names) {
					for _, n := range dupe.Names {
						if _, ok := p[n]; ok && invert || !ok && !invert {
							remove(dryRun, n)
						}
					}
				}

			}
		}
	}

	return nil
}
