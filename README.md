undupes
=======

Undupes is a little command-line tool to find and remove duplicate files. Duplicate file detection is based on the common two-pass approach: we first compare files by size, then by md5 sum. (This is hypothetically unsafe in the event of hash collisions, but probably OK.)

Building
========

    $ go install github.com/danmarg/undupes@latest
    $ ./bin/undupes help

Usage
=====

Undupes has three modes of operation. 

Print
-----
In this mode, Undupes simply prints a list of all duplicate file sets and exits. 

Interactive
-----------
In interactive mode, Undupes prompts the user to resolve every duplicate file set (sorted by size largest-to-smallest):

    Enter parent directory to scan for duplicates in: test
    Found 1 sets of duplicate files
    Total file count: 1
    Total size used: 5B

    Reviewing results:
    For each duplicate fileset, select 'f' to delete all but the first file, 'a' to keep all files, or 'n' (e.g. 2) to delete all except the second file.

    1 of 1  5B:
    1: test/b
    2: test/a

    Keep [F]irst/[a]ll/[n]th?

Automatic
---------
In automatic mode, the user specifies regexps indicating files to keep or remove. By specifying a `--prefer` pattern (and, optionally, a `--over` pattern), one can either remove duplicate files matching a pattern, remove those _not_ matching a pattern, or do likewise only when the other duplicates in the set match a second pattern.

For example, `undupes auto --prefer Gallery --over iPhoto --directory ~/Pictures` will remove photos matching the pattern `iPhoto`, but only when they are duplicated in a file matching `Gallery`. 
