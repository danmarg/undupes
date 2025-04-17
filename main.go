package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"encoding/json"

	"github.com/cenkalti/log"
	"github.com/cheggaaa/pb"
	dupes "github.com/danmarg/undupes/libdupes"
	"github.com/dustin/go-humanize"
	"github.com/urfave/cli"
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
	app.Version = "0.1"
	app.Commands = []cli.Command{
		cli.Command{
			Name:    "interactive",
			Aliases: []string{"i"},
			Usage:   "interactive mode",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:  "directory, d",
					Usage: "directory in which to find duplicates (required)",
				},
				&cli.IntFlag{
					Name:  "v",
					Usage: "log level",
					Value: 3, // Don't print as much in interactive mode.
				},
				&cli.StringFlag{
					Name:  "cache",
					Usage: "cache file",
				},
			},
			Action: func(c *cli.Context) error {
				if err := setLogLevel(c.Int("v")); err != nil {
					return err
				}
				// Pass cache path from flag to runInteractive
				if err := runInteractive(c.Bool("dry_run"), c.StringSlice("directory"), c.String("cache")); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Name:    "print",
			Aliases: []string{"p"},
			Usage:   "print duplicates",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:  "directory, d",
					Usage: "directory in which to find duplicates (required)",
				},
				&cli.StringFlag{
					Name:  "output, o",
					Usage: "output file",
				},
			},
			Action: func(c *cli.Context) error {
				if len(c.StringSlice("directory")) == 0 {
					return fmt.Errorf("--directory required")
				}
				if c.String("output") == "" {
					return fmt.Errorf("--output required")
				}
				// Pass cache path from flag to runPrint (which passes it to getDupesAndPrintSummary)
				// Although print command doesn't have a --cache flag, we might reuse getDupesAndPrintSummary
				// Need to decide if print should *get* a --cache flag or always compute.
				// For now, assume it should compute, pass "" for cachePath.
				// If print should *use* cache, it needs its own --cache flag.
				// Let's add a cache flag to print for consistency.
				// return runPrint(c.StringSlice("directory"), c.String("output"), "") // Old way without cache for print
				return runPrint(c.StringSlice("directory"), c.String("output"), c.String("cache")) // Pass cache
			},
		},
		{
			Name:    "auto",
			Aliases: []string{"a"},
			Usage:   "automatically resolve duplicates",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:  "directory, d",
					Usage: "directory in which to find duplicates (required)",
				},
				&cli.StringFlag{
					Name:  "prefer, p",
					Usage: "prefer to keep files matching this pattern",
				},
				&cli.StringFlag{
					Name:  "over, o",
					Usage: "used with --prefer; will restrict preferred files to those where the duplicate matches --over",
				},
				&cli.BoolFlag{
					Name:  "invert, i",
					Usage: "invert matching logic; preferred files will be prioritized for deletion rather than retention",
				},
				&cli.BoolFlag{
					Name:  "dry_run",
					Usage: "simulate (log but don't delete files)",
				},
				&cli.IntFlag{
					Name:  "v",
					Usage: "log level",
					Value: 1,
				},
				&cli.BoolFlag{
					Name:  "symlink",
					Usage: "create symbolic links instead of deleting duplicate files",
				},
				&cli.StringFlag{
					Name:  "cache",
					Usage: "cache file",
				},
			},
			Action: func(c *cli.Context) error {
				// Check for required arguments.
				if len(c.StringSlice("directory")) == 0 {
					return fmt.Errorf("--directory is required")
				}
				if c.String("prefer") == "" {
					return fmt.Errorf("--prefer is required")
				}
				if err := setLogLevel(c.Int("v")); err != nil {
					return err
				}
				// Compile regexps.
				var prefer, over *regexp.Regexp
				var err error
				if prefer, err = regexp.Compile(c.String("prefer")); err != nil {
					return fmt.Errorf("invalid regexp specified for --prefer")
				}
				if c.String("over") != "" {
					if over, err = regexp.Compile(c.String("over")); err != nil {
						return fmt.Errorf("invalid regexp specified for --over")
					}
				}
				// Do deduplication.
				// Pass cache path from flag to runAutomatic
				return runAutomatic(c.Bool("dry_run"), c.StringSlice("directory"), prefer, over, c.Bool("invert"), c.Bool("symlink"), c.String("cache"))
			},
		},
	}
	app.RunAndExitOnError()
}

func remove(dryRun bool, symlink bool, file string, keep string) {
	if dryRun {
		log.Noticef("DRY RUN: delete %s", file)
	} else {
		if err := os.Remove(file); err != nil {
			log.Warningf("error deleting %s: %v", file, err)
		} else {
			log.Noticef("deleted %s", file)
		}
		if symlink {
			keep, err := filepath.Abs(keep)
			if err != nil {
				log.Warningf("error getting qualified path for %s: %v", keep, err)
			}
			if err := os.Symlink(keep, file); err != nil {
				log.Warningf("error creating symlink %s -> %s: %v", file, keep, err)
			} else {
				log.Noticef("created symlink %s -> %s", file, keep)
			}

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

// getDupesAndPrintSummary finds duplicates, optionally using a cache file.
// It prints a summary of the found duplicates.
func getDupesAndPrintSummary(roots []string, cachePath string) ([]dupes.Info, error) {
	// Check cache first
	if cachePath != "" {
		data, err := os.ReadFile(cachePath)
		if err == nil {
			var cachedDupes []dupes.Info
			err = json.Unmarshal(data, &cachedDupes)
			if err == nil {
				fmt.Printf("Loaded %d duplicate sets from cache: %s\n", len(cachedDupes), cachePath)
				// Print summary from cached data
				fcount := 0
				tsize := int64(0)
				for _, i := range cachedDupes {
					fcount += len(i.Names) - 1
					tsize += i.Size * int64(len(i.Names)-1)
				}
				fmt.Printf("Total file count (cached): %d\n", fcount)
				fmt.Printf("Total size used (cached): %s\n", humanize.Bytes(uint64(tsize)))
				return cachedDupes, nil // Return cached data
			}
			// Log error and fall through to recompute if unmarshal fails
			log.Warningf("Failed to unmarshal cache file %s: %v. Recomputing duplicates.", cachePath, err)
		} else if !os.IsNotExist(err) {
			// Log error and fall through to recompute if read fails for reasons other than non-existence
			log.Warningf("Failed to read cache file %s: %v. Recomputing duplicates.", cachePath, err)
		}
		// If file doesn't exist (os.IsNotExist), we also fall through to compute.
	}

	// --- Compute Duplicates ---
	fmt.Printf("Indexing...")
	var b *pb.ProgressBar
	// Rename 'dupes' to 'computedDupes' to avoid confusion with cachedDupes
	computedDupes, err := dupes.Dupes(roots, func(cur int, outof int) {
		if b == nil {
			b = pb.StartNew(outof)
		}
		b.Set(cur)
	})
	// Note: 'err' here is from dupes.Dupes
	if err != nil {
		return computedDupes, err // Return immediately if Dupes failed
	}
	if b != nil {
		b.Finish()
	}
	// --- Print Summary ---
	fcount := 0
	tsize := int64(0)
	// Iterate over computedDupes
	for _, i := range computedDupes {
		fcount += len(i.Names) - 1
		tsize += i.Size * int64(len(i.Names)-1)
	}

	fmt.Printf("\rFound %d sets of duplicate files\n", len(computedDupes))
	fmt.Printf("Total file count: %d\n", fcount)
	fmt.Printf("Total size used: %s\n", humanize.Bytes(uint64(tsize)))

	// --- Save to Cache (if path specified and we computed) ---
	// This block is only reached if we didn't return early with cached data.
	if cachePath != "" {
		jsonData, marshalErr := json.MarshalIndent(computedDupes, "", "  ")
		if marshalErr != nil {
			// Log marshalling error, but still return computed dupes below
			log.Warningf("Failed to marshal duplicates to JSON: %v", marshalErr)
		} else {
			writeErr := os.WriteFile(cachePath, jsonData, 0644) // Use 0644 permissions
			if writeErr != nil {
				// Log writing error, but still return computed dupes below
				log.Warningf("Failed to write cache file %s: %v", cachePath, writeErr)
			} else {
				fmt.Printf("Saved %d duplicate sets to cache: %s\n", len(computedDupes), cachePath)
			}
		}
	}

	// Return computedDupes and the error from dupes.Dupes (which might be nil)
	return computedDupes, err
}


func runInteractive(dryRun bool, roots []string, cachePath string) error {
	var err error
	if len(roots) == 0 {
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
		roots = []string{root}
	}
	// Pass cachePath down
	d, err := getDupesAndPrintSummary(roots, cachePath)
	if err != nil {
		return err
	}
	dupes := d // Assign to avoid potential unused variable if cache load works

	fmt.Printf("\nReviewing results:\nFor each duplicate fileset, select 'f' to delete all but the first file, 'a' to keep all files, or 'n' (e.g. 2) to delete all except the second file.\n")

	for i, dupe := range dupes {
		keep := -1
		names := ""
		for j, n := range dupe.Names {
			names += fmt.Sprintf("%d: %s\n", j+1, n)
		}
		_, err := getInput(fmt.Sprintf("\n%d of %d  %s:\n%s\nKeep [F]irst/[a]ll/[n]th? ", i+1, len(dupes), humanize.Bytes(uint64(dupe.Size*int64(len(dupe.Names)-1))), names), func(v string) bool {
			switch strings.ToLower(v) {
			case "f", "":
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
					remove(dryRun, false, n, dupe.Names[keep])
				}
			}
		}
	}
	return nil
}

func runPrint(roots []string, output string, cachePath string) error {
	// Pass cachePath down
	d, err := getDupesAndPrintSummary(roots, cachePath)
	if err != nil {
		return err
	}
	dupes := d

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
		l := fmt.Sprintf("%s * %d => %s\n", humanize.Bytes(uint64(dupe.Size)), len(dupe.Names), strings.Join(dupe.Names, ", "))
		if f != nil {
			f.Write([]byte(l))
		} else {
			fmt.Printf(l)
		}
	}
	return nil
}

func runAutomatic(dryRun bool, roots []string, prefer *regexp.Regexp, over *regexp.Regexp, invert bool, symlink bool, cachePath string) error {
	// Pass cachePath down
	d, err := getDupesAndPrintSummary(roots, cachePath)
	if err != nil {
		return err
	}
	dupes := d

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
			var keep string
			for k := range p {
				keep = k
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
							remove(dryRun, symlink, n, keep)
						}
					} else {
						for n := range o {
							remove(dryRun, symlink, n, keep)
						}
					}
				}
			} else {
				// If over is not specified, keep only the preferred, but (for the case of --invert) only when preferred is not everything.
				if len(p) < len(dupe.Names) {
					// Determine the source (preferred) file for symlinking.
					var source string
					for preferred := range p {
						source = preferred
						break // Assuming only one preferred file when len(p) < len(dupe.Names)
					}
					for _, n := range dupe.Names {
						if _, ok := p[n]; ok && invert || !ok && !invert {
							remove(dryRun, symlink, n, source)
						}
					}
				}

			}
		}
	}

	return nil
}
