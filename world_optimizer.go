package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"mc-world-trimmer/chunk"

	"github.com/Tnze/go-mc/save/region"
	"github.com/dustin/go-humanize"
	"github.com/spf13/afero"
)

type WorldOptimizer struct {
	Source            Source
	AnyWorldFound     bool
	ComputeHeightMaps bool
	ComputeLowMaps    bool
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
	for _, file := range readdir {
		switch file.Name() {
		case "level.dat":
			levelFound = true
		case "region":
			if file.IsDir() {
				regionFound = true
			}
		}
	}
	if levelFound && regionFound {
		return o.optimize(dir)
	}
	return nil
}

func (o *WorldOptimizer) optimize(dir string) error {
	o.AnyWorldFound = true
	o.log(dir, "optimize...")
	if err := o.processChunks(dir); err != nil {
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
	if !o.ComputeLowMaps {
		if err := o.removeFileIfExists(filepath.Join(dir, "lowmap.bin")); err != nil {
			return err
		}
	}
	return nil
}

func (o *WorldOptimizer) processChunks(dir string) error {
	var worldSize uint64
	var newWorldSize uint64

	lowmaps := make(map[ChunkPos][]byte)

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
		if !strings.HasSuffix(path, ".mca") {
			continue
		}

		open, err := o.fs().Open(path)
		if err != nil {
			return fmt.Errorf("%s region file read: %w", path, err)
		}
		rg, err := region.Load(open)
		if err != nil {
			return fmt.Errorf("%s region load: %w", path, err)
		}

		removedChunks := make(map[ChunkPos]bool)
		updatedChunks := make(map[ChunkPos]*chunk.Chunk_1_8_8)
		numChunks := 0
		for cx := 0; cx < 32; cx++ {
			for cz := 0; cz < 32; cz++ {
				if !rg.ExistSector(cx, cz) {
					continue
				}

				var c chunk.Chunk_1_8_8
				if sector, err := rg.ReadSector(cx, cz); err != nil {
					return fmt.Errorf("%s read sector %d,%d: %w", path, cx, cz, err)
				} else if err = c.Load(sector); err != nil {
					return fmt.Errorf("%s read chunk %d,%d: %w", path, cx, cz, err)
				}

				numChunks++

				updated := false
				if c.IsEmpty() {
					removedChunks[ChunkPos{cx, cz}] = true
					continue
				} else if c.Optimize() {
					if c.IsEmpty() {
						removedChunks[ChunkPos{cx, cz}] = true
						continue
					} else {
						updated = true
					}
				}

				if o.ComputeHeightMaps && c.ComputeHeightMap() {
					updated = true
				}

				if o.ComputeLowMaps {
					lowmaps[ChunkPos{int(c.XPos), int(c.ZPos)}] = c.ComputeLowMap()
				}

				if updated {
					updatedChunks[ChunkPos{cx, cz}] = &c
				}
			}
		}

		if len(updatedChunks) > 0 || numChunks > len(removedChunks) && len(removedChunks) > 0 {
			newFile, err := o.fs().Create(path)
			if err != nil {
				return fmt.Errorf("%s create file: %w", path, err)
			}
			replace, err := region.CreateWriter(newFile)
			if err != nil {
				return fmt.Errorf("%s create region: %w", path, err)
			}

			for cx := 0; cx < 32; cx++ {
				for cz := 0; cz < 32; cz++ {
					if !rg.ExistSector(cx, cz) {
						continue
					}
					if _, ok := removedChunks[ChunkPos{cx, cz}]; ok {
						continue
					}

					if c, ok := updatedChunks[ChunkPos{cx, cz}]; ok {
						if data, err := c.Save(); err != nil {
							return fmt.Errorf("%s write chunk %d,%d: %w", path, cx, cz, err)
						} else if err := replace.WriteSector(cx, cz, data); err != nil {
							return fmt.Errorf("%s write sector %d,%d: %w", path, cx, cz, err)
						}
					} else {
						if sector, err := rg.ReadSector(cx, cz); err != nil {
							return fmt.Errorf("%s read sector %d,%d: %w", path, cx, cz, err)
						} else if err := replace.WriteSector(cx, cz, sector); err != nil {
							return fmt.Errorf("%s write sector %d,%d: %w", path, cx, cz, err)
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

		newWorldSize += uint64(file.Size())
	}

	if worldSize != newWorldSize {
		o.log(
			dir, "regions optimized",
			humanize.Bytes(worldSize), "=>", humanize.Bytes(newWorldSize),
			fmt.Sprintf("(%v%%)", math.Round(-(100-100*float64(newWorldSize)/float64(worldSize)))),
		)
	}

	if o.ComputeLowMaps {
		if err = o.saveLowMap(dir, lowmaps); err != nil {
			return err
		}
	}

	return nil
}

func (o *WorldOptimizer) saveLowMap(dir string, lowmap map[ChunkPos][]byte) error {
	sorted := make([]ChunkPos, 0, len(lowmap))
	for pos := range lowmap {
		sorted = append(sorted, pos)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Z == sorted[j].Z {
			return sorted[i].X < sorted[j].X
		}
		return sorted[i].Z < sorted[j].Z
	})

	buf := make([]byte, 0, 4+len(lowmap)*(8+len(lowmap[sorted[0]])))
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(lowmap)))
	for _, pos := range sorted {
		buf = binary.BigEndian.AppendUint32(buf, uint32(pos.X))
		buf = binary.BigEndian.AppendUint32(buf, uint32(pos.Z))
	}
	for _, pos := range sorted {
		buf = append(buf, lowmap[pos]...)
	}

	dest := filepath.Join(dir, "lowmap.bin")
	if ok, err := afero.Exists(o.fs(), dest); ok || err != nil {
		if err != nil {
			return err
		}
		file, err := afero.ReadFile(o.fs(), dest)
		if err != nil {
			return err
		}
		// no need to overwrite
		if bytes.Equal(file, buf) {
			return nil
		}
	}

	return afero.WriteFile(o.fs(), dest, buf, 0644)
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

type ChunkPos struct {
	X int
	Z int
}
