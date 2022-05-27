package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

var overwrite = flag.Bool("o", false, "Overwrite original world")
var suffix = flag.String("s", "_opt", "Suffix for optimized worlds")
var dryRun = flag.Bool("dry", false, "Dry run (no changes on disk)")
var verbose = flag.Bool("v", false, "Verbose logging")

var foundAny = false

func main() {
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		base := filepath.Base(os.Args[0])
		fmt.Fprintln(w, "Usage:")
		fmt.Fprintln(w, " ", base, "[options] path")
		fmt.Fprintln(w, "Examples:")
		fmt.Fprintln(w, " ", base, "-dry .minecraft/saves")
		fmt.Fprintln(w, " ", base, "-o .")
		fmt.Fprintln(w, " ", base, "-o world.zip")
		fmt.Fprintln(w, "Options:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		return
	}

	path := strings.Join(flag.Args(), " ")
	if strings.HasSuffix(path, ".zip") {
		process(NewZipSource(path), true)
		return
	}

	dirsDone := make(map[string]bool)

	// Find plain directories
	fs := afero.NewBasePathFs(afero.NewOsFs(), path)
	plainDirs, err := findWorldDirs(fs)
	if err != nil {
		log.Fatalln(err)
	}
	for _, dir := range plainDirs {
		fullPath := filepath.Join(path, dir)
		if dirsDone[fullPath] {
			continue
		}
		if strings.HasSuffix(dir, *suffix) {
			log.Println("Skip", dir, "as optimized")
			continue
		}
		dirsDone[fullPath] = true
		process(NewDirSource(fullPath), false)
	}

	// Find zip files
	zipFiles, err := findZipFiles(fs)
	if err != nil {
		log.Fatalln(err)
	}
	for _, file := range zipFiles {
		if strings.HasSuffix(file, *suffix+".zip") {
			log.Println("Skip", file, "as optimized")
			continue
		}
		process(NewZipSource(filepath.Join(path, file)), true)
	}

	if !foundAny {
		log.Println("No worlds found in", path)
	}
}

func process(source Source, recursive bool) {
	optimizer := &WorldOptimizer{
		Source: source,
	}
	if err := optimizer.Process(recursive); err != nil {
		log.Fatalln(err)
	}
	if optimizer.AnyWorldFound {
		foundAny = true
	}
	if !*dryRun {
		if err := source.Save(); err != nil {
			log.Fatalln(err)
		}
	}
	if err := source.Close(); err != nil {
		log.Fatalln(err)
	}
}

func findZipFiles(fs afero.Fs) ([]string, error) {
	var files []string
	err := afero.Walk(fs, "", func(path string, f os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".zip") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func findWorldDirs(fs afero.Fs) ([]string, error) {
	var files []string
	err := afero.Walk(fs, "", func(path string, f os.FileInfo, err error) error {
		if filepath.Base(path) == "region" && f.IsDir() {
			files = append(files, filepath.Dir(path))
		}
		return nil
	})
	return files, err
}
