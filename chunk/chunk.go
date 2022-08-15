package chunk

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Tnze/go-mc/nbt"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
)

type RegionLevel_1_8_8 struct {
	Level Chunk_1_8_8
}

type Chunk_1_8_8 struct {
	Entities         nbt.RawMessage
	Sections         []Section
	TileEntities     nbt.RawMessage
	InhabitedTime    int64
	LastUpdate       int64
	LightPopulated   byte
	TerrainPopulated byte
	V                int32 `nbt:",omitempty"`
	XPos             int32 `nbt:"xPos"`
	ZPos             int32 `nbt:"zPos"`
	Biomes           []byte
	HeightMap        []int32

	sectionCache []*Section
}

type Section struct {
	Y          byte
	SkyLight   []byte `nbt:",omitempty"`
	BlockLight []byte
	Blocks     []byte
	Data       []byte
	Add        []byte `nbt:",omitempty"`
}

func (c *Chunk_1_8_8) Load(data []byte) (err error) {
	var r io.Reader = bytes.NewReader(data[1:])

	switch data[0] {
	default:
		err = errors.New("unknown compression")
	case 1:
		reader := gzipReaderPool.Get()
		if reader == nil {
			r, err = gzip.NewReader(r)
		} else {
			r = reader.(*gzip.Reader)
			err = reader.(*gzip.Reader).Reset(r)
		}
		defer func() {
			gzipReaderPool.Put(reader)
		}()
	case 2:
		reader := zlibReaderPool.Get()
		if reader == nil {
			r, err = zlib.NewReader(r)
		} else {
			r = reader.(io.Reader)
			err = reader.(zlib.Resetter).Reset(r, nil)
		}
		defer func() {
			zlibReaderPool.Put(reader)
		}()
	}

	if err != nil {
		return err
	}

	level := RegionLevel_1_8_8{Level: *c}
	_, err = nbt.NewDecoder(r).Decode(&level)
	*c = level.Level
	return
}

func (c *Chunk_1_8_8) Save() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(2)

	var w io.Writer
	writer := zlibWriterPool.Get()
	if writer == nil {
		w = zlib.NewWriter(&buf)
	} else {
		w = writer.(*zlib.Writer)
		writer.(*zlib.Writer).Reset(&buf)
	}
	defer func() {
		zlibWriterPool.Put(w)
	}()

	level := RegionLevel_1_8_8{Level: *c}
	err := nbt.NewEncoder(w).Encode(level, "")
	if err != nil {
		return nil, fmt.Errorf("write chunk: %w", err)
	}
	err = w.(*zlib.Writer).Flush()
	return buf.Bytes(), err
}

func (c *Chunk_1_8_8) IsEmpty() bool {
	if len(c.Sections) == 0 && len(c.Entities.Data) == 5 && len(c.TileEntities.Data) == 5 {
		return true
	}
	return false
}

func (c *Chunk_1_8_8) Optimize() bool {
	before := len(c.Sections)
	for i := len(c.Sections) - 1; i >= 0; i-- {
		if !isZero(c.Sections[i].Blocks) {
			continue
		}
		if !isZero(c.Sections[i].Add) {
			continue
		}
		c.Sections = append(c.Sections[:i], c.Sections[i+1:]...)
	}
	return before != len(c.Sections)
}

func (c *Chunk_1_8_8) ComputeHeightMap() bool {
	maxY := 0
	for i := range c.Sections {
		if c.Sections[i].Y > byte(maxY&15) {
			maxY = int(c.Sections[i].Y)<<4 + 16
		}
	}

	changed := false
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := maxY; y > 0; y-- {
				id, _ := c.GetType(x, y-1, z)
				if !transparent_1_8_8[id] {
					if c.HeightMap[z<<4|x] != int32(y) {
						c.HeightMap[z<<4|x] = int32(y)
						changed = true
					}
					break
				}
			}
		}
	}
	return changed
}

func (c *Chunk_1_8_8) GetType(x, y, z int) (int, byte) {
	if y > 255 {
		return 0, 0
	}
	if c.sectionCache == nil {
		c.sectionCache = make([]*Section, 16)
		for i := range c.Sections {
			c.sectionCache[int(c.Sections[i].Y)] = &c.Sections[i]
		}
	}
	sec := c.sectionCache[y>>4]
	if sec == nil {
		return 0, 0
	}
	idx := (y&15)<<8 | (z&15)<<4 | (x & 15)

	id := int(sec.Blocks[idx])
	if sec.Add != nil {
		id = int(nibbleGet(sec.Add, idx))<<8 | id
	}
	data := nibbleGet(sec.Data, idx)
	return id, data
}

func nibbleGet(data []byte, idx int) byte {
	return data[idx>>1] >> ((idx & 1) << 2)
}

var dummyBytes [1 << 16]byte // 65536

func isZero(data []byte) bool {
	return bytes.Equal(data, dummyBytes[:len(data)])
}

var gzipReaderPool sync.Pool
var zlibReaderPool sync.Pool
var zlibWriterPool sync.Pool
