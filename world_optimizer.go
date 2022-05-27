package main

import (
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/afero"
	"mc-world-trimmer/region"
)

type WorldOptimizer struct {
	Source        Source
	AnyWorldFound bool
}

func (o *WorldOptimizer) Process(recursive bool) error {
	if recursive {
		matches, _ := findWorldDirs(o.fs())
		for _, m := range matches {
			if err := o.checkWorldCandidate(m); err != nil {
				return err
			}
		}
	} else {
		return o.checkWorldCandidate("")
	}
	return nil
}

func (o *WorldOptimizer) checkWorldCandidate(dir string) error {
	if ok, err := afero.IsDir(o.fs(), dir); err != nil {
		return err
	} else if !ok {
		return nil
	}
	readdir, err := afero.ReadDir(o.fs(), dir)
	if err != nil {
		return err
	}

	levelFound := false
	regionFound := false
	uidFound := false
	for _, file := range readdir {
		switch file.Name() {
		case "level.dat":
			levelFound = true
		case "uid.dat":
			uidFound = true
		case "region":
			if file.IsDir() {
				regionFound = true
			}
		}
	}
	if levelFound && uidFound && regionFound {
		return o.optimize(dir)
	}
	return nil
}

func (o *WorldOptimizer) optimize(dir string) error {
	o.AnyWorldFound = true
	o.log(dir, "optimize...")
	if err := o.optimizeChunks(dir); err != nil {
		return err
	}
	if err := o.deleteUselessFiles(dir); err != nil {
		return err
	}
	return nil
}

func (o *WorldOptimizer) deleteUselessFiles(dir string) error {
	if err := o.removeDirIfExists(filepath.Join(dir, "playerdata")); err != nil {
		return err
	}
	if err := o.removeDirIfExists(filepath.Join(dir, "stats")); err != nil {
		return err
	}
	if err := o.removeFileIfExists(filepath.Join(dir, "level.dat_old")); err != nil {
		return err
	}
	if err := o.removeFileIfExists(filepath.Join(dir, "session.lock")); err != nil {
		return err
	}
	return nil
}

func (o *WorldOptimizer) optimizeChunks(dir string) error {
	var worldSize uint64
	var newWorldSize uint64

	regionDirPath := filepath.Join(dir, "region")
	regionFiles, err := afero.ReadDir(o.fs(), regionDirPath)
	if err != nil {
		return err
	}
	for _, file := range regionFiles {
		worldSize += uint64(file.Size())
		path := filepath.Join(regionDirPath, file.Name())
		if strings.HasSuffix(path, ".mcr") {
			if exists, err := afero.Exists(o.fs(), path[:len(path)-4]+".mca"); err != nil {
				return err
			} else if !exists {
				continue
			}
			if err := o.fs().Remove(path); err != nil {
				return err
			}
			continue
		}

		open, err := o.fs().Open(path)
		if err != nil {
			return err
		}
		rg, err := region.Load(open)
		if err != nil {
			return fmt.Errorf("%s region load: %w", path, err)
		}

		removedChunks := make(map[int]bool)
		updatedChunks := make(map[int]*region.Chunk_1_8_8)
		numChunks := 0
		for cx := 0; cx < 32; cx++ {
			for cz := 0; cz < 32; cz++ {
				if !rg.ExistSector(cx, cz) {
					continue
				}

				var chunk region.Chunk_1_8_8
				if sector, err := rg.ReadSector(cx, cz); err != nil {
					return fmt.Errorf("%s read sector %d,%d: %w", path, cx, cz, err)
				} else if err = chunk.Load(sector); err != nil {
					return fmt.Errorf("%s read chunk %d,%d: %w", path, cx, cz, err)
				}

				numChunks++

				if chunk.IsEmpty() {
					removedChunks[cx*1000+cz] = true
				} else if chunk.Optimize() {
					if chunk.IsEmpty() {
						removedChunks[cx*1000+cz] = true
					} else {
						updatedChunks[cx*1000+cz] = &chunk
					}
				}
			}
		}

		if len(updatedChunks) > 0 || numChunks > len(removedChunks) && len(removedChunks) > 0 {
			newFile, err := o.fs().Create(path)
			if err != nil {
				return err
			}
			replace, err := region.Create(newFile)
			if err != nil {
				return err
			}

			for cx := 0; cx < 32; cx++ {
				for cz := 0; cz < 32; cz++ {
					if !rg.ExistSector(cx, cz) {
						continue
					}
					if _, ok := removedChunks[cx*1000+cz]; ok {
						continue
					}

					if chunk, ok := updatedChunks[cx*1000+cz]; ok {
						if data, err := chunk.Save(); err != nil {
							return fmt.Errorf("%s write chunk %d,%d: %w", path, cx, cz, err)
						} else if err := replace.WriteSector(cx, cz, data); err != nil {
							return fmt.Errorf("%s write sector %d,%d: %w", path, cx, cz, err)
						}
					} else {
						sector, _ := rg.ReadSector(cx, cz)
						if err := replace.WriteSector(cx, cz, sector); err != nil {
							return err
						}
					}
				}
			}
			if err := replace.PadToFullSector(); err != nil {
				return err
			}
			if err := replace.Close(); err != nil {
				return err
			}
			stat, _ := newFile.Stat()
			if *verbose {
				o.log(dir, file.Name(), "updated",
					humanize.Bytes(uint64(file.Size())), "to", humanize.Bytes(uint64(stat.Size())),
				)
			}
			newWorldSize += uint64(stat.Size())
			_ = open.Close()
			continue
		}

		_ = open.Close()

		if numChunks == len(removedChunks) {
			if *verbose {
				o.log(dir, file.Name(), "removed", humanize.Bytes(uint64(file.Size())))
			}
			if err := o.fs().Remove(path); err != nil {
				return err
			}
			continue
		}

		if *verbose {
			//o.log(dir, file.Name(), "unchanged", humanize.Bytes(uint64(file.Size())))
		}
		newWorldSize += uint64(file.Size())
	}

	if worldSize != newWorldSize {
		o.log(
			dir, "regions optimized",
			humanize.Bytes(worldSize), "=>", humanize.Bytes(newWorldSize),
			fmt.Sprintf("(%v%%)", math.Round(-(100-100*float64(newWorldSize)/float64(worldSize)))),
		)
	}

	return nil
}

func (o *WorldOptimizer) removeDirIfExists(dir string) error {
	if exists, err := afero.DirExists(o.fs(), dir); err != nil {
		return err
	} else if exists {
		o.log(dir, "dir removed")
		return o.fs().RemoveAll(dir)
	}
	return nil
}

func (o *WorldOptimizer) removeFileIfExists(file string) error {
	if exists, err := afero.Exists(o.fs(), file); err != nil {
		return err
	} else if exists {
		o.log(file, "removed")
		return o.fs().Remove(file)
	}
	return nil
}

func (o *WorldOptimizer) fs() afero.Fs {
	return o.Source.Fs()
}

func (o *WorldOptimizer) log(args ...string) {
	log.Println(o.Source.Name(), args)
}
