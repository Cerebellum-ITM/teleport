// Package bindetect identifies the target OS of a local executable
// by sniffing the first bytes of the file (ELF, Mach-O, PE).
package bindetect

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

type OS string

const (
	Linux   OS = "linux"
	MacOS   OS = "macos"
	Windows OS = "windows"
)

// ErrUnknownFormat is returned when the file's magic bytes do not match
// any recognised executable format.
var ErrUnknownFormat = errors.New("not a recognised executable binary (no ELF/Mach-O/PE magic)")

var (
	magicELF = []byte{0x7F, 0x45, 0x4C, 0x46}
	magicPE  = []byte{0x4D, 0x5A}

	machOMagics = []uint32{
		0xFEEDFACE, 0xFEEDFACF, // 32/64-bit Mach-O (BE host)
		0xCEFAEDFE, 0xCFFAEDFE, // little-endian counterparts
		0xCAFEBABE, // fat binary
	}
)

// Detect reads the first 8 bytes of path and returns the matching OS.
func Detect(path string) (OS, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var head [8]byte
	n, err := f.Read(head[:])
	if err != nil || n < 4 {
		return "", ErrUnknownFormat
	}

	if bytes.HasPrefix(head[:], magicELF) {
		return Linux, nil
	}
	if bytes.HasPrefix(head[:], magicPE) {
		return Windows, nil
	}
	be := binary.BigEndian.Uint32(head[:4])
	for _, m := range machOMagics {
		if be == m {
			return MacOS, nil
		}
	}

	return "", ErrUnknownFormat
}

// Valid reports whether s is one of the recognised OS strings.
func Valid(s string) bool {
	switch OS(s) {
	case Linux, MacOS, Windows:
		return true
	}
	return false
}
