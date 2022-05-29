package main

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	kpzip "github.com/klauspost/compress/zip"

	"github.com/spf13/afero"
	"github.com/spf13/afero/zipfs"
)

type ZipSource struct {
	z       *zip.ReadCloser
	file    string
	overlay *OverlayFs
}

func NewZipSource(file string) *ZipSource {
	z, err := zip.OpenReader(file)
	if err != nil {
		log.Fatalln(fmt.Errorf("open zip file %s %w", file, err))
	}
	s := &ZipSource{
		z:       z,
		file:    file,
		overlay: NewOverlayFs(zipfs.New(&z.Reader)),
	}
	return s
}

func (s *ZipSource) Name() string {
	return s.file
}

func (s *ZipSource) Fs() afero.Fs {
	return s.overlay
}

func (s *ZipSource) Save() error {
	if s.overlay.IsChanged() {
		var outFile *os.File
		var err error
		if *overwrite {
			outFile, err = os.CreateTemp("", "world*.zip")
		} else {
			split := strings.Split(s.file, ".")
			barename := strings.Join(split[:len(split)-1], ".")
			outFile, err = os.Create(barename + *suffix + ".zip")
		}
		if err != nil {
			return err
		}
		zw := kpzip.NewWriter(outFile)
		err = afero.Walk(s.overlay, "", func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if s.overlay.IsRemoved(path) != nil {
				return nil
			}
			header, err := kpzip.FileInfoHeader(info)
			if err != nil {
				return fmt.Errorf("getting info for file %s: %w", info.Name(), err)
			}
			header.Name = filepath.ToSlash(filepath.Clean(path))
			if header.Name == "." {
				return nil
			}
			if info.IsDir() {
				if !strings.HasSuffix(header.Name, "/") {
					header.Name += "/" // required
				}
				header.Method = kpzip.Store
			} else {
				header.Method = kpzip.Deflate
			}

			create, err := zw.CreateHeader(header)
			if err != nil || info.IsDir() {
				return err
			}

			if file, err := afero.ReadFile(s.overlay, path); err != nil {
				return err
			} else if _, err = create.Write(file); err != nil {
				return err
			}
			return nil
		})
		_ = zw.Close()
		_ = outFile.Close()
		if err != nil {
			_ = os.Remove(outFile.Name())
			return err
		}
		if err := s.Close(); err != nil {
			return err
		}
		if *overwrite {
			stat, err := os.Stat(s.file)
			if err != nil {
				return err
			}
			mode := stat.Mode()
			if err = os.Rename(outFile.Name(), s.file); err != nil {
				return err
			}
			if err = os.Chmod(s.file, mode); err != nil {
				return err
			}
			log.Println("Saved file", s.file)
		} else {
			log.Println("Saved file", outFile.Name())
		}
	}
	return nil
}

func (s *ZipSource) Close() (err error) {
	if s.z != nil {
		err = s.z.Close()
		s.z = nil
	}
	return
}
