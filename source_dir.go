package main

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

type DirSource struct {
	dir     string
	overlay *OverlayFs
}

func NewDirSource(dir string) *DirSource {
	return &DirSource{
		dir:     dir,
		overlay: NewOverlayFs(afero.NewBasePathFs(afero.NewOsFs(), dir)),
	}
}

func (s *DirSource) Name() string {
	return s.dir
}

func (s *DirSource) Fs() afero.Fs {
	return s.overlay
}

func (s *DirSource) Save() error {
	if s.overlay.IsChanged() {
		var err error
		var out string
		if *overwrite {
			out, err = os.MkdirTemp("", "world")
			defer func(path string) {
				err := os.RemoveAll(path)
				if err != nil {
					log.Println(err)
				}
			}(out)
		} else {
			out = s.dir + *suffix
			if err = os.RemoveAll(out); err != nil {
				return err
			}
			err = os.Mkdir(out, 0666)
		}
		out = filepath.Clean(out)
		if err != nil {
			return err
		}

		err = afero.Walk(s.overlay, "", func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if s.overlay.IsRemoved(path) != nil {
				return nil
			}
			dest := filepath.Join(out, path)
			if info.IsDir() {
				if dest != out {
					return os.Mkdir(dest, 0666)
				}
				return nil
			}
			file, err := afero.ReadFile(s.overlay, path)
			if err != nil {
				return err
			}
			return os.WriteFile(dest, file, 0666)
		})
		if err != nil {
			return err
		}

		if *overwrite {
			stat, err := os.Stat(s.dir)
			if err != nil {
				return err
			}
			mode := stat.Mode()
			if err := os.RemoveAll(s.dir); err != nil {
				return err
			}
			if err = os.Rename(out, s.dir); err != nil {
				return err
			}
			if err = os.Chmod(s.dir, mode); err != nil {
				return err
			}
			log.Println("Overwrited dir", s.dir)
		} else {
			log.Println("Created dir", out)
		}
	}
	return nil
}

func (s *DirSource) Close() error {
	return nil
}
