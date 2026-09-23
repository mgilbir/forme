// Command maketables regenerates the tables a make target names, and is the one
// way the Makefile runs a generator.
//
//	go run ./cmd/maketables -D UCD=testdata/ucd -D UNICODE_VERSION=17.0.0 ... casing
//
// What each target regenerates, and from what, is cmd/internal/tables.Manifest;
// the -D values are the Makefile's variables, which say where the inputs are
// and which pin they are at. cmd/regenerate_test.go runs the same manifest
// through the same code and compares the result with the committed files, so
// what `make casing` would write and what the test checks cannot be two things.
//
// It fails loudly and writes nothing on any error. Every table a target names
// is generated before any is written; a generator that fails, a variable with
// no value, a target the manifest does not know, stop the run with a non-zero
// exit and leave every committed file as it was. And each file is replaced by
// renaming a complete temporary file over it, never by truncating it first —
// the recipes this replaced wrote "go run ... > table.go", which emptied the
// table before the generator ran, and a loop over several of them without
// "set -e" went on to the next and reported success.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mgilbir/forme/cmd/internal/tables"
)

// defines is the repeatable -D NAME=VALUE flag.
type defines map[string]string

func (d defines) String() string { return fmt.Sprint(map[string]string(d)) }

func (d defines) Set(s string) error {
	name, value, ok := strings.Cut(s, "=")
	if !ok || name == "" {
		return fmt.Errorf("%q is not NAME=VALUE", s)
	}
	if _, dup := d[name]; dup {
		return fmt.Errorf("%s is defined twice", name)
	}
	d[name] = value
	return nil
}

func main() {
	vars := defines{}
	flag.Var(vars, "D", "a Makefile variable, NAME=VALUE; repeatable")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: maketables -D NAME=VALUE... <target>...")
		os.Exit(2)
	}
	if err := run(".", tables.Manifest, vars, flag.Args(), tables.Replace, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "maketables:", err)
		os.Exit(1)
	}
}

// run regenerates every table of the manifest the targets name, under root,
// and hands each to write.
func run(root string, manifest []tables.Table, vars map[string]string, targets []string,
	write func(path string, data []byte) error, log io.Writer) error {
	var chosen []tables.Table
	for _, target := range targets {
		n := 0
		for _, t := range manifest {
			if t.Target == target {
				chosen = append(chosen, t)
				n++
			}
		}
		if n == 0 {
			return fmt.Errorf("the manifest has no table for the target %q; nothing was written", target)
		}
	}
	var missing []string
	for _, name := range tables.Names(chosen) {
		if vars[name] == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("no value for %s; the Makefile passes every variable "+
			"the manifest names, so the two have drifted; nothing was written",
			strings.Join(missing, ", "))
	}

	// All of them first, and only then any of them written, so that a generator
	// that fails leaves every table the target names as it was.
	generated := make([][]byte, len(chosen))
	var errs []string
	for i, t := range chosen {
		data, err := t.Generate(root, vars)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		generated[i] = data
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d of %d tables could not be generated, and nothing was written:\n%s",
			len(errs), len(chosen), strings.Join(errs, "\n"))
	}
	for i, t := range chosen {
		if err := write(root+"/"+t.Out, generated[i]); err != nil {
			return fmt.Errorf("writing %s: %v", t.Out, err)
		}
		fmt.Fprintf(log, "wrote %s (cmd/%s)\n", t.Out, t.Generator)
	}
	return nil
}
