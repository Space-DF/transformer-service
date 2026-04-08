package common

// Big-endian readers.

func I32BE(b []byte, off int) int32 {
	return int32(b[off])<<24 | int32(b[off+1])<<16 | int32(b[off+2])<<8 | int32(b[off+3])
}

func U32BE(b []byte, off int) uint32 {
	return uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])
}

func U24BE(b []byte, off int) uint32 {
	return uint32(b[off])<<16 | uint32(b[off+1])<<8 | uint32(b[off+2])
}

func U16BE(b []byte, off int) uint16 {
	return uint16(b[off])<<8 | uint16(b[off+1])
}

func I16BE(b []byte, off int) int16 {
	u := U16BE(b, off)
	if u > 0x7FFF {
		return int16(int32(u) - 0x10000) // #nosec G115 -- safe, checked conversion
	}
	return int16(u) // #nosec G115 -- safe, checked conversion
}

// ReadInt24BE reads a signed 24-bit big-endian integer with sign extension.
// Used by Cayenne LPP GPS channels (tbeam).
func ReadInt24BE(b []byte, off int) int32 {
	v := int32(b[off])<<16 | int32(b[off+1])<<8 | int32(b[off+2])
	if v&0x800000 != 0 {
		v |= ^int32(0xFFFFFF)
	}
	return v
}

// Little-endian readers.

func I32LE(b []byte, off int) int32 {
	return int32(b[off]) | int32(b[off+1])<<8 | int32(b[off+2])<<16 | int32(b[off+3])<<24
}

func U32LE(b []byte, off int) uint32 {
	return uint32(b[off]) | uint32(b[off+1])<<8 | uint32(b[off+2])<<16 | uint32(b[off+3])<<24
}

func I16LE(b []byte, off int) int16 {
	return int16(b[off]) | int16(b[off+1])<<8
}
