package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/afero"
)

type OverlayFs struct {
	source          afero.Fs
	changes         afero.Fs
	overlay         afero.Fs
	deletedFiles    map[string]bool
	deletedPrefixes []string
}

func NewOverlayFs(fs afero.Fs) *OverlayFs {
	s := &OverlayFs{
		source:          fs,
		changes:         afero.NewMemMapFs(),
		deletedFiles:    make(map[string]bool),
		deletedPrefixes: []string{},
	}
	s.overlay = afero.NewCopyOnWriteFs(s.source, s.changes)
	return s
}

func (r *OverlayFs) IsChanged() bool {
	list, _ := afero.ReadDir(r.changes, "")
	if len(list) > 0 || len(r.deletedFiles) > 0 || len(r.deletedPrefixes) > 0 {
		return true
	}
	return false
}

func (r *OverlayFs) ReadDir(name string) ([]os.FileInfo, error) {
	return afero.ReadDir(r.overlay, name)
}

func (r *OverlayFs) Chtimes(n string, a, m time.Time) error {
	return syscall.EPERM
}

func (r *OverlayFs) Chmod(n string, m os.FileMode) error {
	return syscall.EPERM
}

func (r *OverlayFs) Chown(n string, uid, gid int) error {
	return syscall.EPERM
}

func (r *OverlayFs) Name() string {
	return "OverlayFs"
}

func (r *OverlayFs) Stat(name string) (os.FileInfo, error) {
	return r.overlay.Stat(name)
}

func (r *OverlayFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	if lsf, ok := r.overlay.(afero.Lstater); ok {
		return lsf.LstatIfPossible(name)
	}
	fi, err := r.Stat(name)
	return fi, false, err
}

func (r *OverlayFs) SymlinkIfPossible(oldname, newname string) error {
	return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: afero.ErrNoSymlink}
}

func (r *OverlayFs) ReadlinkIfPossible(name string) (string, error) {
	if srdr, ok := r.overlay.(afero.LinkReader); ok {
		return srdr.ReadlinkIfPossible(name)
	}

	return "", &os.PathError{Op: "readlink", Path: name, Err: afero.ErrNoReadlink}
}

func (r *OverlayFs) Rename(o, n string) error {
	return syscall.EPERM
}

func (r *OverlayFs) RemoveAll(p string) error {
	justDeleted := false
	if exists, _ := afero.Exists(r.source, p); exists {
		clean := filepath.Clean(p)
		alreadyDeleted := false
		for _, prefix := range r.deletedPrefixes {
			if prefix == clean {
				alreadyDeleted = true
				break
			}
		}
		if !alreadyDeleted {
			r.deletedPrefixes = append(r.deletedPrefixes, clean)
			justDeleted = true
		}
	}
	if err := r.overlay.RemoveAll(p); err != nil {
		if justDeleted {
			return nil
		}
		return err
	}
	return nil
}

func (r *OverlayFs) Remove(n string) error {
	err0 := r.overlay.Remove(n)

	if exists, _ := afero.Exists(r.source, n); exists {
		if err := r.IsRemoved(n); err != nil {
			return err
		}
		r.deletedFiles[filepath.Clean(n)] = true
		return nil
	}

	return err0
}

func (r *OverlayFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return r.overlay.OpenFile(name, flag, perm)
}

func (r *OverlayFs) Open(n string) (afero.File, error) {
	return r.overlay.Open(n)
}

func (r *OverlayFs) Mkdir(n string, p os.FileMode) error {
	// Тут надо зафиксить удаление
	return r.overlay.Mkdir(n, p)
}

func (r *OverlayFs) MkdirAll(n string, p os.FileMode) error {
	// И тут тоже
	return r.overlay.MkdirAll(n, p)
}

func (r *OverlayFs) Create(n string) (afero.File, error) {
	file, err := r.overlay.Create(n)
	delete(r.deletedFiles, filepath.Clean(n))
	return file, err
}

func (r *OverlayFs) IsRemoved(path string) error {
	if _, ok := r.deletedFiles[path]; ok {
		return &os.PathError{Op: "cc fileRemoved", Path: path, Err: afero.ErrFileNotFound}
	}
	for _, prefix := range r.deletedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return &os.PathError{Op: "cc dirRemoved", Path: path, Err: afero.ErrFileNotFound}
		}
	}
	return nil
}
