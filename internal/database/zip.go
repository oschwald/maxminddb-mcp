package database

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
)

// Check counts and offsets before archive/zip allocates the directory entries.
func checkZIPDirectory(ctx context.Context, source *os.File, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	const endSize = 22
	tail := make([]byte, min(size, endSize+65535))
	if _, err := source.ReadAt(tail, size-int64(len(tail))); err != nil {
		return err
	}
	if len(tail) < endSize {
		return zip.ErrFormat
	}
	index := bytes.LastIndex(tail[:len(tail)-endSize+4], []byte("PK\x05\x06"))
	if index < 0 {
		return zip.ErrFormat
	}
	end := tail[index:]
	le := binary.LittleEndian
	if len(end) != endSize+int(le.Uint16(end[20:])) {
		return zip.ErrFormat
	}
	directoryEnd := uint64(size - int64(len(end)))
	count := uint64(le.Uint16(end[10:]))
	directorySize := uint64(le.Uint32(end[12:]))
	offset := uint64(le.Uint32(end[16:]))
	if count == 0xffff || directorySize == 0xffffffff || offset == 0xffffffff {
		var locator [20]byte
		if _, err := source.ReadAt(locator[:], int64(directoryEnd)-20); err != nil {
			return err
		}
		if string(locator[:4]) != "PK\x06\x07" || le.Uint32(locator[4:]) != 0 ||
			le.Uint32(locator[16:]) != 1 {
			return zip.ErrFormat
		}
		zip64Offset := le.Uint64(locator[8:])
		if directoryEnd < 76 || zip64Offset > directoryEnd-76 {
			return zip.ErrFormat
		}
		var end64 [56]byte
		if _, err := source.ReadAt(end64[:], int64(zip64Offset)); err != nil {
			return err
		}
		if string(end64[:4]) != "PK\x06\x06" ||
			le.Uint64(end64[4:]) != directoryEnd-20-zip64Offset-12 {
			return zip.ErrFormat
		}
		count = le.Uint64(end64[32:])
		directorySize = le.Uint64(end64[40:])
		offset = le.Uint64(end64[48:])
		directoryEnd = zip64Offset
	}
	if count > 1024 {
		return errors.New("archive contains more than 1024 entries")
	}
	if offset > directoryEnd || directorySize != directoryEnd-offset {
		return zip.ErrFormat
	}
	var header [46]byte
	var entries uint64
	for offset < directoryEnd {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entries >= 1024 {
			return errors.New("archive contains more than 1024 entries")
		}
		if directoryEnd-offset < uint64(len(header)) {
			return zip.ErrFormat
		}
		if _, err := source.ReadAt(header[:], int64(offset)); err != nil {
			return err
		}
		if string(header[:4]) != "PK\x01\x02" {
			return zip.ErrFormat
		}
		offset += uint64(len(header)) + uint64(le.Uint16(header[28:])) +
			uint64(le.Uint16(header[30:])) + uint64(le.Uint16(header[32:]))
		entries++
	}
	if offset != directoryEnd || entries != count {
		return zip.ErrFormat
	}
	return nil
}
