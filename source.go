package main

import (
	"github.com/spf13/afero"
)

type Source interface {
	Name() string
	Fs() afero.Fs
	Save() error
	Close() error
}
