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
var recursive = flag.Bool("r", false, "Recursive search for worlds")
var heightMap = flag.Bool("hm", false, "Recalculate height maps")
var lowMap = flag.Bool("lm", false, "Compute low maps")

var foundAny = false

func main() {
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		base := filepath.Base(os.Args[0])
		fmt.Fprintln(w, "Usage:")
		fmt.Fprintln(w, " ", base, "[options] path")
		fmt.Fprintln(w, "Examples:")
		fmt.Fprintln(w, " ", base, "-r -dry .minecraft/saves")
		fmt.Fprintln(w, " ", base, "-r -o .")
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

	if *recursive {
		// Find plain directories
		abspath, err := filepath.Abs(path)
		if err != nil {
			log.Fatalln(err)
		}
		fs := afero.NewBasePathFs(afero.NewOsFs(), abspath)
		plainDirs, err := findWorldDirs(fs)
		if err != nil {
			log.Fatalln(err)
		}
		dirsDone := make(map[string]bool)
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
	} else {
		process(NewDirSource(path), false)
	}

	if !foundAny {
		log.Println("No worlds found in", path)
	}
}

func process(source Source, recursive bool) {
	optimizer := &WorldOptimizer{
		Source:            source,
		ComputeHeightMaps: *heightMap,
		ComputeLowMaps:    *lowMap,
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
		if err != nil {
			return err
		}
		if f.IsDir() && f.Name() == ".git" {
			return filepath.SkipDir
		}
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
		if err != nil {
			return err
		}
		if f.IsDir() && f.Name() == ".git" {
			return filepath.SkipDir
		}
		if filepath.Base(path) == "region" && f.IsDir() {
			files = append(files, filepath.Dir(path))
		}
		return nil
	})
	return files, err
}
