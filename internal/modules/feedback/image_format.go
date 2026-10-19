package feedback

import (
	"bufio"
	"encoding/binary"
	"hash/crc32"
	"io"
)

// Validate PNG chunk framing/CRC and JPEG marker framing through the end marker.
// Pixel decompression is left to the image viewer; no size-based pixel allocation occurs here.
func inspectEncodedImage(source io.Reader, mime string) error {
	r := bufio.NewReader(source)
	if mime == "image/png" {
		if _, err := io.CopyN(io.Discard, r, 8); err != nil {
			return err
		}
		data := false
		for {
			var header [8]byte
			if _, err := io.ReadFull(r, header[:]); err != nil {
				return err
			}
			n := int64(binary.BigEndian.Uint32(header[:4]))
			kind := string(header[4:])
			crc := crc32.NewIEEE()
			_, _ = crc.Write(header[4:])
			if _, err := io.CopyN(crc, r, n); err != nil {
				return err
			}
			var sum [4]byte
			if _, err := io.ReadFull(r, sum[:]); err != nil {
				return err
			}
			if crc.Sum32() != binary.BigEndian.Uint32(sum[:]) {
				return invalidImages
			}
			if kind == "IDAT" {
				data = true
			}
			if kind == "IEND" {
				if !data || n != 0 {
					return invalidImages
				}
				return nil
			}
		}
	}
	if _, err := io.CopyN(io.Discard, r, 2); err != nil {
		return err
	}
	scanned := false
	entropy := false
	for {
		if entropy {
			for {
				_, err := r.ReadSlice(0xff)
				if err == bufio.ErrBufferFull {
					continue
				}
				if err != nil {
					return err
				}
				break
			}
		} else {
			v, err := r.ReadByte()
			if err != nil {
				return err
			}
			if v != 0xff {
				return invalidImages
			}
		}
		marker, err := r.ReadByte()
		if err != nil {
			return err
		}
		for marker == 0xff {
			marker, err = r.ReadByte()
			if err != nil {
				return err
			}
		}
		if entropy && (marker == 0 || marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		entropy = false
		if marker == 0xd9 {
			if !scanned {
				return invalidImages
			}
			return nil
		}
		if marker == 0 || marker == 0xd8 {
			return invalidImages
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		var length [2]byte
		if _, err = io.ReadFull(r, length[:]); err != nil {
			return err
		}
		n := int64(binary.BigEndian.Uint16(length[:]))
		if n < 2 {
			return invalidImages
		}
		if _, err = io.CopyN(io.Discard, r, n-2); err != nil {
			return err
		}
		if marker == 0xda {
			scanned = true
			entropy = true
		}
	}
}
