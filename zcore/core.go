package zcore

import "encoding/binary"

type Core struct {
	bytes                            []uint8
	Version                          uint8
	FlagByte1                        uint8
	StatusBarTimeBased               bool
	ReleaseNumber                    uint16
	PagedMemoryBase                  uint16
	FirstInstruction                 uint16
	DictionaryBase                   uint16
	ObjectTableBase                  uint16
	GlobalVariableBase               uint16
	StaticMemoryBase                 uint16
	AbbreviationTableBase            uint16
	FileChecksum                     uint16
	InterpreterNumber                uint8
	InterpreterVersion               uint8
	ScreenHeightLines                uint8
	ScreenWidthChars                 uint8
	ScreenWidthUnits                 uint16
	ScreenHeightUnits                uint16
	FontHeight                       uint8
	FontWidth                        uint8
	RoutinesOffset                   uint16
	StringOffset                     uint16
	DefaultBackgroundColorNumber     uint8
	DefaultForegroundColorNumber     uint8
	TerminatingCharTableBase         uint16
	OutputStream3Width               uint16
	StandardRevisionNumber           uint16
	AlternativeCharSetBaseAddress    uint16
	ExtensionTableBaseAddress        uint16
	PlayerLoginName                  []uint8
	UnicodeExtensionTableBaseAddress uint16
}

func LoadCore(bytes []uint8) Core {
	bytes[0x1e] = 0x6 // Interpreter number - IBM PC chosen as closest match
	bytes[0x1f] = 0x1 // Interpreter version - nobody cares

	// Set screen dimensions - games may use these for layout calculations
	// Using typical terminal dimensions (80x25 characters, 1x1 units per char)
	bytes[0x20] = 25  // Screen height (lines)
	bytes[0x21] = 80  // Screen width (characters)
	bytes[0x22] = 0   // Screen width (units) - high byte
	bytes[0x23] = 80  // Screen width (units) - low byte (same as chars for text-only)
	bytes[0x24] = 0   // Screen height (units) - high byte
	bytes[0x25] = 25  // Screen height (units) - low byte
	bytes[0x26] = 1   // Font height (units)
	bytes[0x27] = 1   // Font width (units)

	// Claim that this interpreter supports v1.2 of the spec (aspirational!)
	bytes[0x32] = 0x1
	bytes[0x33] = 0x2

	// Set the flags to say what is available in this interpreter
	if bytes[0] <= 3 {
		bytes[1] |= 0b0010_0000 // Only flag to set is the "split screen available one"
	} else {
		// Flags: colors (0x01), bold (0x04), italic (0x08), split screen (0x20)
		// NOT claiming: pictures (0x02), fixed-width default (0x10), timed input (0x80)
		bytes[1] |= 0b0010_1101
	}

	// Parse the extension table for any interesting information we want
	extensionTableBaseAddress := binary.BigEndian.Uint16(bytes[0x36:0x38])
	unicodeExtensionTableBaseAddress := uint16(0)
	if extensionTableBaseAddress != 0 {
		unicodeExtensionTableBaseAddress = binary.BigEndian.Uint16(bytes[extensionTableBaseAddress+6 : extensionTableBaseAddress+8])
	}

	return Core{
		bytes:                            bytes,
		Version:                          bytes[0x00],
		FlagByte1:                        bytes[0x01],
		StatusBarTimeBased:               bytes[0x01]&0b0000_0010 == 0b0000_0010,
		ReleaseNumber:                    binary.BigEndian.Uint16(bytes[0x02:0x04]),
		PagedMemoryBase:                  binary.BigEndian.Uint16(bytes[0x04:0x06]),
		FirstInstruction:                 binary.BigEndian.Uint16(bytes[0x06:0x08]),
		DictionaryBase:                   binary.BigEndian.Uint16(bytes[0x08:0x0a]),
		ObjectTableBase:                  binary.BigEndian.Uint16(bytes[0x0a:0x0c]),
		GlobalVariableBase:               binary.BigEndian.Uint16(bytes[0x0c:0x0e]),
		StaticMemoryBase:                 binary.BigEndian.Uint16(bytes[0x0e:0x10]),
		AbbreviationTableBase:            binary.BigEndian.Uint16(bytes[0x18:0x1a]),
		FileChecksum:                     binary.BigEndian.Uint16(bytes[0x1c:0x1e]),
		InterpreterNumber:                bytes[0x1e],
		InterpreterVersion:               bytes[0x1f],
		ScreenHeightLines:                bytes[0x20],
		ScreenWidthChars:                 bytes[0x21],
		ScreenWidthUnits:                 binary.BigEndian.Uint16(bytes[0x22:0x24]),
		ScreenHeightUnits:                binary.BigEndian.Uint16(bytes[0x24:0x26]),
		FontHeight:                       bytes[0x26],
		FontWidth:                        bytes[0x27],
		RoutinesOffset:                   binary.BigEndian.Uint16(bytes[0x28:0x2a]),
		StringOffset:                     binary.BigEndian.Uint16(bytes[0x2a:0x2c]),
		DefaultBackgroundColorNumber:     bytes[0x2c],
		DefaultForegroundColorNumber:     bytes[0x2d],
		TerminatingCharTableBase:         binary.BigEndian.Uint16(bytes[0x2e:0x30]),
		OutputStream3Width:               binary.BigEndian.Uint16(bytes[0x30:0x32]),
		StandardRevisionNumber:           binary.BigEndian.Uint16(bytes[0x32:0x34]),
		AlternativeCharSetBaseAddress:    binary.BigEndian.Uint16(bytes[0x34:0x36]),
		ExtensionTableBaseAddress:        extensionTableBaseAddress,
		PlayerLoginName:                  bytes[0x38:0x40],
		UnicodeExtensionTableBaseAddress: unicodeExtensionTableBaseAddress,
	}
}

// func (z *ZMachine) flagByte2() uint8         { return bytes[0x10] }
// func (z *ZMachine) flagByte3() uint8         { return bytes[0x11] }
// func (z *ZMachine) serialCode() []uint8      { return bytes[0x12:0x18] }

func (core *Core) FileLength() uint16 {
	var divisor uint16
	version := core.Version
	switch {
	case version <= 3:
		divisor = 2
	case version <= 5:
		divisor = 4
	default:
		divisor = 8
	}
	return binary.BigEndian.Uint16(core.bytes[0x1a:0x1c]) * divisor
}

func (core *Core) SetDefaultBackgroundColorNumber(color uint8) {
	core.bytes[0x2c] = color
	core.DefaultBackgroundColorNumber = color
}
func (core *Core) SetDefaultForegroundColorNumber(color uint8) {
	core.bytes[0x2c] = color
	core.DefaultForegroundColorNumber = color
}

func (core *Core) ReadZByte(address uint32) uint8 {
	return core.bytes[address]
}

func (core *Core) ReadHalfWord(address uint32) uint16 {
	return binary.BigEndian.Uint16(core.bytes[address : address+2])
}

func (core *Core) ReadLongWord(address uint32) uint64 {
	return binary.BigEndian.Uint64(core.bytes[address : address+8])
}

func (core *Core) ReadSlice(startAddress uint32, endAddress uint32) []uint8 {
	return core.bytes[startAddress:endAddress]
}

func (core *Core) WriteZByte(address uint32, value uint8) {
	// TODO - Lots of the memory is read only, need to add validation here
	core.bytes[address] = value
}

func (core *Core) WriteHalfWord(address uint32, value uint16) {
	// TODO - Lots of the memory is read only, need to add validation here
	binary.BigEndian.PutUint16(core.bytes[address:address+2], value)
}

func (core *Core) WriteWord(address uint32, value uint32) {
	// TODO - Lots of the memory is read only, need to add validation here
	binary.BigEndian.PutUint32(core.bytes[address:address+4], value)
}

func (core *Core) MemoryLength() uint32 {
	return uint32(len(core.bytes))
}
