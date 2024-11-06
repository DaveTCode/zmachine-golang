package zmachine

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/davetcode/goz/dictionary"
	"github.com/davetcode/goz/zobject"
	"github.com/davetcode/goz/zstring"
	"github.com/davetcode/goz/ztable"
)

type StatusBar struct {
	PlaceName   string
	Score       int
	Moves       int
	IsTimeBased bool
}

type Quit bool

type StateChangeRequest int

const (
	WaitForInput StateChangeRequest = iota
	Running      StateChangeRequest = iota
)

type RoutineType int

const (
	function  RoutineType = iota
	procedure RoutineType = iota
	interrupt RoutineType = iota
)

type MemoryStreamData struct {
	baseAddress uint32
	ptr         uint32
}

type Streams struct {
	Screen           bool
	Transcript       bool
	Memory           bool
	MemoryStreamData []MemoryStreamData
	CommandScript    bool
}

type ZMachine struct {
	callStack     CallStack
	Memory        []uint8
	dictionary    *dictionary.Dictionary
	screenModel   ScreenModel
	streams       Streams
	rng           rand.Rand
	Alphabets     *zstring.Alphabets
	outputChannel chan<- interface{}
	inputChannel  <-chan string
	UndoStates    InMemorySaveStateCache
}

func (z *ZMachine) Version() uint8           { return z.Memory[0] }
func (z *ZMachine) flagByte1() uint8         { return z.Memory[0x01] }
func (z *ZMachine) statusBarTimeBased() bool { return (z.flagByte1() & 0b0000_0010) != 0 }
func (z *ZMachine) releaseNumber() uint16    { return binary.BigEndian.Uint16(z.Memory[0x02:0x04]) }

// func (z *ZMachine) pagedMemoryBase() uint16  { return binary.BigEndian.Uint16(z.Memory[0x04:0x06]) }
func (z *ZMachine) firstInstruction() uint16 { return binary.BigEndian.Uint16(z.Memory[0x06:0x08]) }
func (z *ZMachine) dictionaryBase() uint16   { return binary.BigEndian.Uint16(z.Memory[0x08:0x0a]) }
func (z *ZMachine) ObjectTableBase() uint16  { return binary.BigEndian.Uint16(z.Memory[0x0a:0x0c]) }
func (z *ZMachine) globalVariableBase() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x0c:0x0e])
}
func (z *ZMachine) staticMemoryBase() uint16 { return binary.BigEndian.Uint16(z.Memory[0x0e:0x10]) }

// func (z *ZMachine) flagByte2() uint8         { return z.Memory[0x10] }
// func (z *ZMachine) flagByte3() uint8         { return z.Memory[0x11] }
// func (z *ZMachine) serialCode() []uint8      { return z.Memory[0x12:0x18] }
func (z *ZMachine) AbbreviationTableBase() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x18:0x1a])
}
func (z *ZMachine) fileLength() uint16 {
	var divisor uint16
	version := z.Version()
	switch {
	case version <= 3:
		divisor = 2
	case version <= 5:
		divisor = 4
	default:
		divisor = 8
	}
	return binary.BigEndian.Uint16(z.Memory[0x1a:0x1c]) * divisor
}
func (z *ZMachine) fileChecksum() uint16                        { return binary.BigEndian.Uint16(z.Memory[0x1c:0x1e]) }
func (z *ZMachine) interpreterNumber() uint8                    { return z.Memory[0x1e] }
func (z *ZMachine) interpreterVersion() uint8                   { return z.Memory[0x1f] }
func (z *ZMachine) screenHeightLines() uint8                    { return z.Memory[0x20] }
func (z *ZMachine) screenWidthChars() uint8                     { return z.Memory[0x21] }
func (z *ZMachine) screenWidthUnits() uint16                    { return binary.BigEndian.Uint16(z.Memory[0x22:0x24]) }
func (z *ZMachine) screenHeightUnits() uint16                   { return binary.BigEndian.Uint16(z.Memory[0x24:0x26]) }
func (z *ZMachine) fontHeight() uint8                           { return z.Memory[0x26] }
func (z *ZMachine) fontWidth() uint8                            { return z.Memory[0x27] }
func (z *ZMachine) routinesOffset() uint16                      { return binary.BigEndian.Uint16(z.Memory[0x28:0x2a]) }
func (z *ZMachine) stringOffset() uint16                        { return binary.BigEndian.Uint16(z.Memory[0x2a:0x2c]) }
func (z *ZMachine) setDefaultBackgroundColorNumber(color uint8) { z.Memory[0x2c] = color }
func (z *ZMachine) defaultBackgroundColorNumber() uint8         { return z.Memory[0x2c] }
func (z *ZMachine) setDefaultForegroundColorNumber(color uint8) { z.Memory[0x2c] = color }
func (z *ZMachine) defaultForegroundColorNumber() uint8         { return z.Memory[0x2d] }
func (z *ZMachine) terminatingCharTableBase() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x2e:0x30])
}
func (z *ZMachine) outputStream3Width() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x30:0x32])
}
func (z *ZMachine) standardRevisionNumber() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x32:0x34])
}
func (z *ZMachine) alternativeCharSetBaseAddress() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x34:0x36])
}
func (z *ZMachine) extensionTableBaseAddress() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x36:0x38])
}
func (z *ZMachine) playerLoginName() []uint8 { return z.Memory[0x38:0x40] }

func (z *ZMachine) packedAddress(originalAddress uint32, isZString bool) uint32 {
	switch {
	case z.Version() < 4:
		return 2 * originalAddress
	case z.Version() < 6:
		return 4 * originalAddress
	case z.Version() < 8:
		offset := z.routinesOffset()
		if isZString {
			offset = z.stringOffset()
		}
		return 4*originalAddress + 8*uint32(offset)
	case z.Version() == 8:
		return 8 * originalAddress
	default:
		panic("Invalid rom version")
	}
}

func (z *ZMachine) readIncPC(frame *CallStackFrame) uint8 {
	v := z.readByte(frame.pc)
	frame.pc++
	return v
}

func (z *ZMachine) readHalfWordIncPC(frame *CallStackFrame) uint16 {
	v := z.readHalfWord(frame.pc)
	frame.pc += 2
	return v
}

func (z *ZMachine) readByte(address uint32) uint8 {
	return z.Memory[address]
}

func (z *ZMachine) writeByte(address uint32, value uint8) {
	// TODO - Lots of the memory is read only, need to add validation here
	z.Memory[address] = value
}

func (z *ZMachine) readHalfWord(address uint32) uint16 {
	return binary.BigEndian.Uint16(z.Memory[address : address+2])
}

func (z *ZMachine) writeHalfWord(address uint32, value uint16) {
	// TODO - Lots of the memory is read only, need to add validation here
	binary.BigEndian.PutUint16(z.Memory[address:address+2], value)
}

func (z *ZMachine) readVariable(variable uint8, indirect bool) uint16 {
	currentCallFrame := z.callStack.peek()

	switch {
	case variable == 0: // Magic stack variable
		if len(currentCallFrame.routineStack) == 0 {
			panic("Attempt to read from empty routine stack")
		}

		// "In the seven opcodes that take indirect variable references (inc, dec, inc_chk, dec_chk, load, store, pull),
		// an indirect reference to the stack pointer does not push or pull the top item of the stack -
		// it is read or written in place." - Verified with praxix tests
		if indirect {
			return currentCallFrame.peek()
		} else {
			return currentCallFrame.pop()
		}
	case variable < 16: // Routine local variables

		if variable-1 >= uint8(len(currentCallFrame.locals)) {
			panic("Attempt to access non-existing local variable")
		}

		return currentCallFrame.locals[variable-1]
	default: // Global variables
		return z.readHalfWord(uint32(z.globalVariableBase() + 2*(uint16(variable)-16)))
	}
}

func (z *ZMachine) writeVariable(variable uint8, value uint16, indirect bool) {
	currentCallFrame := z.callStack.peek()

	switch {
	case variable == 0: // Magic stack variable
		// Indirect writes happen in place at the top of the stack
		if indirect {
			_ = currentCallFrame.pop()
		}

		currentCallFrame.push(value)
	case variable < 16: // Routine local variables
		if variable-1 >= uint8(len(currentCallFrame.locals)) {
			panic("Attempt to access non-existing local variable")
		}

		currentCallFrame.locals[variable-1] = value
	default: // Global variables
		z.writeHalfWord(uint32(z.globalVariableBase()+2*(uint16(variable)-16)), value)
	}
}

func LoadRom(rom []uint8, inputChannel <-chan string, outputChannel chan<- interface{}) *ZMachine {
	machine := ZMachine{
		Memory:        rom,
		inputChannel:  inputChannel,
		outputChannel: outputChannel,
		streams: Streams{
			Screen:        true,
			Transcript:    false,
			Memory:        false,
			CommandScript: false,
		},
		rng: *rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	machine.Memory[0x1e] = 0x6 // Interpreter number - IBM PC chosen as closest match
	machine.Memory[0x1f] = 0x1 // Interpreter version - nobody cares
	// TODO - Should really set screen height/width etc here but they can change so want to handle with channels and status updates from the ui properly

	// Claim that this interpreter supports v1.2 of the spec (aspirational!)
	machine.Memory[0x32] = 0x1
	machine.Memory[0x33] = 0x2

	// Set the flags to say what is available in this interpreter
	if machine.Version() <= 3 {
		machine.Memory[1] |= 0b0010_0000 // Only flag to set is the "split screen available one"
	} else {
		machine.Memory[1] |= 0b1010_1101 // No pictures or fixed width (we only have one font)
	}

	// Load custom alphabets on v5+
	machine.Alphabets = zstring.LoadAlphabets(machine.Version(), rom, machine.alternativeCharSetBaseAddress())

	// TODO - Is the dictionary static? If not shouldn't cache like this
	machine.dictionary = dictionary.ParseDictionary(machine.Memory[machine.dictionaryBase():], uint32(machine.dictionaryBase()), machine.Version(), machine.Alphabets, machine.AbbreviationTableBase())

	machine.setDefaultBackgroundColorNumber(uint8(Black))
	machine.setDefaultForegroundColorNumber(uint8(White))
	machine.screenModel = newScreenModel(Black, White)

	// V6+ uses a packed address and a routine for the initial function
	if machine.Version() == 6 {
		packedAddress := machine.packedAddress(uint32(machine.firstInstruction()), false)

		machine.callStack.push(CallStackFrame{
			pc:     packedAddress + 1,
			locals: make([]uint16, machine.Memory[packedAddress]),
		})
	} else {
		machine.callStack.push(CallStackFrame{
			pc:     uint32(machine.firstInstruction()),
			locals: make([]uint16, 0),
		})
	}

	return &machine
}

func (z *ZMachine) call(opcode *Opcode, routineType RoutineType) {
	routineAddress := z.packedAddress(uint32(opcode.operands[0].Value(z)), false)

	// Special case, if routine address is 0 then no call is made and 0 is stored in the return address
	if routineAddress == 0 {
		if routineType == function {
			z.writeVariable(z.readIncPC(z.callStack.peek()), 0, false)
		}

		return
	}

	localVariableCount := z.readByte(routineAddress)
	routineAddress++

	locals := make([]uint16, localVariableCount)

	for i := 0; i < int(localVariableCount); i++ {
		if i+1 < len(opcode.operands) {
			// Value passed to routine, override default
			locals[i] = opcode.operands[i+1].Value(z)
		} else {
			// No value passed to routine, use default
			if z.Version() < 5 {
				locals[i] = z.readHalfWord(routineAddress)
			}
		}

		if z.Version() < 5 {
			routineAddress += 2
		}
	}

	z.callStack.push(CallStackFrame{
		pc:              routineAddress,
		locals:          locals,
		routineStack:    make([]uint16, 0),
		routineType:     routineType, // TODO - Not really sure what this is, v3+ only
		numValuesPassed: len(opcode.operands) - 1,
		framePointer:    0, // TODO - Only used for try/catch in later versions
	})
}

func (z *ZMachine) handleBranch(frame *CallStackFrame, result bool) {
	branchArg1 := z.readIncPC(frame)

	branchReversed := (branchArg1>>7)&1 == 0
	singleByte := (branchArg1>>6)&1 == 1
	offset := int32(branchArg1 & 0b11_1111)

	if !singleByte {
		offset = int32(int16((uint16(branchArg1&0b11_1111)<<8|uint16(z.readIncPC(frame)))<<2) >> 2)
	}

	if result != branchReversed {
		if offset == 0 {
			z.retValue(0)
		} else if offset == 1 {
			z.retValue(1)
		} else {
			destination := uint32(int32(frame.pc) + offset - 2)
			frame.pc = destination
		}
	}
}

type word struct {
	bytes             []uint8
	startingLocation  uint32
	dictionaryAddress uint16
}

func tokeniseSingleWord(bytes []uint8, wordStartPtr uint32, dictionary *dictionary.Dictionary, version uint8, alphabets *zstring.Alphabets) word {
	runes := []rune(string(bytes))
	zstr := zstring.Encode(runes, version, alphabets)

	dictionaryAddress := dictionary.Find(zstr)

	return word{
		bytes:             bytes,
		startingLocation:  wordStartPtr,
		dictionaryAddress: dictionaryAddress,
	}
}

func (z *ZMachine) Tokenise(baddr1 uint32, baddr2 uint32, dictionary *dictionary.Dictionary, leaveWordsBlank bool) {
	bytesRead := 0
	words := make([]word, 0)
	startingLocation := baddr1 + 1 // Skip byte which has max length of string in it
	chrCount := uint32(0)
	if z.Version() >= 5 {
		chrCount = uint32(z.readByte(startingLocation))
		startingLocation++
	}
	currentLocation := startingLocation

	for _, chr := range z.Memory[startingLocation:] {
		if (z.Version() < 5 && chr == 0) || (z.Version() >= 5 && currentLocation-startingLocation >= chrCount) {
			words = append(words, tokeniseSingleWord(z.Memory[startingLocation:currentLocation], startingLocation, dictionary, z.Version(), z.Alphabets))
			break
		}

		if chr == ' ' { // space is always a separator
			words = append(words, tokeniseSingleWord(z.Memory[startingLocation:currentLocation], startingLocation, dictionary, z.Version(), z.Alphabets))
			startingLocation = currentLocation + 1
		} else {
			for _, separator := range z.dictionary.Header.InputCodes {
				if chr == separator {
					words = append(words, tokeniseSingleWord(z.Memory[startingLocation:currentLocation], startingLocation, dictionary, z.Version(), z.Alphabets))
					words = append(words, tokeniseSingleWord(z.Memory[currentLocation:currentLocation+1], startingLocation, dictionary, z.Version(), z.Alphabets))
					startingLocation = currentLocation + 1
					break
				}
			}
		}

		currentLocation += 1
		bytesRead += 1
	}

	if z.readByte(baddr2) < uint8(len(words)) {
		panic("Error to have more words than allowed in the buffer here")
	}

	parseBufferPtr := baddr2 + 1
	z.writeByte(parseBufferPtr, uint8(len(words)))
	parseBufferPtr += 1
	for _, word := range words {
		z.writeHalfWord(parseBufferPtr, word.dictionaryAddress)
		z.writeByte(parseBufferPtr+2, uint8(len(word.bytes)))
		z.writeByte(parseBufferPtr+3, uint8(word.startingLocation-baddr1))

		parseBufferPtr += 4
	}
}

func (z *ZMachine) retValue(val uint16) {
	oldFrame := z.callStack.pop()
	newFrame := z.callStack.peek()

	if oldFrame.routineType == function {
		destination := z.readIncPC(newFrame)
		z.writeVariable(destination, val, false)
	}
}

func (z *ZMachine) RemoveObject(objId uint16) {
	object := zobject.GetObject(objId, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
	if object.Parent != 0 {
		oldParent := zobject.GetObject(object.Parent, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())

		// Remove from old location in the sibling chain
		if oldParent.Child == object.Id {
			// First child case
			oldParent.SetChild(object.Sibling, z.Version(), z.Memory)
		} else {
			// Non-first child case - in theory can't have a sibling if no parent so no need to do this if parent == 0
			currObjId := oldParent.Child
			for {
				if currObjId == 0 {
					break
				}

				currObj := zobject.GetObject(currObjId, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
				if currObj.Sibling == object.Id {
					currObj.SetSibling(object.Sibling, z.Version(), z.Memory)
					break
				} else {
					currObjId = currObj.Sibling
				}
			}
		}

		object.SetParent(0, z.Version(), z.Memory)
	}

	object.SetSibling(0, z.Version(), z.Memory)
}

func (z *ZMachine) MoveObject(objId uint16, newParent uint16) {
	object := zobject.GetObject(objId, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
	destinationObject := zobject.GetObject(newParent, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())

	// Don't bother moving an object if the parent already matches (the algorithm below breaks then anyway!)
	if object.Parent == destinationObject.Id {
		return
	}

	// Detach it from it's current place in the tree
	z.RemoveObject(object.Id)

	// Set new location in the tree
	object.SetSibling(destinationObject.Child, z.Version(), z.Memory)
	object.SetParent(destinationObject.Id, z.Version(), z.Memory)
	destinationObject.SetChild(object.Id, z.Version(), z.Memory)
}

func (z *ZMachine) appendText(s string) {
	if z.streams.Memory {
		currentMemoryStream := &z.streams.MemoryStreamData[len(z.streams.MemoryStreamData)-1]
		for _, r := range s {
			z.Memory[currentMemoryStream.ptr] = uint8(r)
			currentMemoryStream.ptr++
		}

		// 7.1.2.2
		// Output stream 3 is unusual in that, while it is selected, no text is sent to any other output streams which are selected. (However, they remain selected.)
		return
	}

	if z.streams.Screen {
		z.outputChannel <- s

		// If writing to the upper window we need to update the screen model and
		// reflect the change in cursor position
		if !z.screenModel.LowerWindowActive {
			lines := strings.Split(s, "\n")
			z.screenModel.UpperWindowCursorY += len(lines)
			z.screenModel.UpperWindowCursorX += len(lines[len(lines)-1])
			z.outputChannel <- z.screenModel
		}
	}

	if z.streams.Transcript {
		panic("TODO - Not implemented transcript")
	}

	if z.streams.CommandScript {
		panic("TODO - Not implemented command script stream")
	}
}

func (z *ZMachine) read(opcode *Opcode) {
	if z.Version() <= 3 { // TODO - Not really sure if this is true
		currentLocation := zobject.GetObject(z.readVariable(16, false), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
		z.outputChannel <- StatusBar{
			PlaceName:   currentLocation.Name,
			Score:       int(z.readVariable(17, false)),
			Moves:       int(z.readVariable(18, false)),
			IsTimeBased: z.statusBarTimeBased(),
		}
	}

	// In V5+ a custom set of terminating characters can be stored in memory
	validTerminators := []uint8{'\n'}
	if z.Version() >= 5 {
		if z.terminatingCharTableBase() != 0 {
			//panic("TODO - Don't use this yet so panic and fix if you find a story file with this set")
			terminatingChrPtr := z.terminatingCharTableBase()
			for {
				b := z.readByte(uint32(terminatingChrPtr))
				if b == 0 {
					break
				} else if (b >= 129 && b <= 154) || (b >= 252 && b <= 254) {
					validTerminators = append(validTerminators, b)
				} else if b == 255 { // Special case means "all function keys are terminators"
					validTerminators = []uint8{'\n', 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143, 144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 252, 253, 254}
					break
				}

				terminatingChrPtr++
			}
		}
	}

	// TODO - Handle timed interrupts of the read function
	// TODO - Somehow let UI know how many chars to accept
	z.outputChannel <- WaitForInput
	rawText := <-z.inputChannel
	textBufferPtr := opcode.operands[0].Value(z)
	parseBufferPtr := opcode.operands[1].Value(z)

	rawTextBytes := []byte(strings.ToLower(rawText))

	bufferSize := z.readByte(uint32(textBufferPtr))
	textBufferPtr++

	// Skip bytes already in the buffer on v5+
	if z.Version() >= 5 {
		existingBytes := z.readByte(uint32(textBufferPtr))
		textBufferPtr += 1 + uint16(existingBytes)
	}

	ix := 0
	for {
		if ix > int(bufferSize) || ix >= len(rawTextBytes) { // TODO - Not 100% sure on whether this is >= or some other off by one value. Docs are unclear
			break // Too many characters provided
		}

		chr := rawTextBytes[ix]

		if (chr >= 32 && chr <= 126) || (chr >= 155 && chr <= 251) {
			z.writeByte(uint32(textBufferPtr+uint16(ix)), chr)
		} else {
			z.writeByte(uint32(textBufferPtr+uint16(ix)), 32)
		}

		ix++
	}

	// Terminate with a null byte
	z.writeByte(uint32(textBufferPtr+uint16(ix)), 0)

	// Need to store the number of bytes in total in v5+ as that's used to determine end point of the string
	if z.Version() >= 5 {
		z.writeByte(uint32(opcode.operands[0].Value(z)+1), uint8(ix))
	}

	// TODO - Can this ever really be zero?
	if parseBufferPtr != 0 {
		z.Tokenise(uint32(opcode.operands[0].Value(z)), uint32(parseBufferPtr), z.dictionary, false)
	}

	if z.Version() >= 5 {
		z.writeVariable(z.readIncPC(z.callStack.peek()), 13, false) // TODO - Should be the typed terminating char
	}
}

func (z *ZMachine) Run() {
	// Initialise whatever is listening by sending inital versions of the screen model
	z.outputChannel <- z.screenModel

	for {
		if !z.StepMachine() {
			break
		}
	}

	z.outputChannel <- Quit(true)
}

// Debugging information, show last 100 program counter addresses
var pcHistory = make([]Opcode, 100)
var pcHistoryPtr = 0

func (z *ZMachine) StepMachine() bool {
	if z.callStack.peek().pc == 0x6c8b {
		pcHistoryPtr = pcHistoryPtr + 1 - 1
	}

	opcode := ParseOpcode(z)
	frame := z.callStack.peek()

	pcHistory[pcHistoryPtr] = opcode
	pcHistoryPtr = (pcHistoryPtr + 1) % 100

	if z.Memory[0x6c27] == 0x9e {
		pcHistoryPtr = pcHistoryPtr + 1 - 1
	}

	switch opcode.operandCount {
	case OP0:
		switch opcode.opcodeNumber {
		case 0: // RTRUE
			z.retValue(1)

		case 1: // RFALSE
			z.retValue(0)

		case 2: // PRINT
			text, bytesRead := zstring.Decode(z.Memory, frame.pc, uint32(len(z.Memory)), z.Version(), z.Alphabets, z.AbbreviationTableBase(), false)
			frame.pc += bytesRead
			z.appendText(text)

		case 3: // PRINT_RET
			text, bytesRead := zstring.Decode(z.Memory, frame.pc, uint32(len(z.Memory)), z.Version(), z.Alphabets, z.AbbreviationTableBase(), false)
			frame.pc += bytesRead
			z.appendText(text)
			z.appendText("\n")
			z.retValue(1)

		case 8: // RET_POPPED
			v := frame.pop()
			z.retValue(v)

		case 10: // QUIT
			return false

		case 11: // NEWLINE
			z.appendText("\n")

		case 13: // VERIFY
			checksum := z.fileChecksum()
			fileLength := z.fileLength()
			actualChecksum := uint16(0)

			for ix := 0x40; ix < int(fileLength); ix++ {
				actualChecksum += uint16(z.Memory[ix])
			}

			z.handleBranch(frame, checksum == actualChecksum || true) // TODO - Verify doesn't really work but also not clear why we'd ever want to fail a verify test

		case 15: // PIRACY
			z.handleBranch(frame, true) // Interpreters are asked to be gullible and to unconditionally branch

		default:
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
		}

	case OP1:
		switch opcode.opcodeNumber {
		case 0: // JZ
			z.handleBranch(frame, opcode.operands[0].Value(z) == 0)

		case 1: // GET_SIBLING
			sibling := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()).Sibling
			z.writeVariable(z.readIncPC(frame), sibling, false)

			z.handleBranch(frame, sibling != 0)

		case 2: // GET_CHILD
			child := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()).Child
			z.writeVariable(z.readIncPC(frame), child, false)

			z.handleBranch(frame, child != 0)

		case 3: // GET_PARENT
			z.writeVariable(z.readIncPC(frame), zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()).Parent, false)

		case 4: // GET_PROP_LEN
			addr := opcode.operands[0].Value(z)
			z.writeVariable(z.readIncPC(frame), zobject.GetPropertyLength(z.Memory, uint32(addr), z.Version()), false)

		case 5: // INC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable, true)+1, true)

		case 6: // DEC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable, true)-1, true)

		case 7: // PRINT_ADDR
			address := opcode.operands[0].Value(z)
			str, _ := zstring.Decode(z.Memory, uint32(address), uint32(len(z.Memory)), z.Version(), z.Alphabets, z.AbbreviationTableBase(), false)
			z.appendText(str)

		case 8: // CALL_1S
			z.call(&opcode, function)

		case 9: // REMOVE_OBJ
			z.RemoveObject(opcode.operands[0].Value(z))

		case 10: // PRINT_OBJ
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			z.appendText(obj.Name)

		case 11: // RET
			v := opcode.operands[0].Value(z)
			z.retValue(v)

		case 12: // JUMP
			offset := int16(opcode.operands[0].Value(z))
			destination := uint32(int32(frame.pc) + int32(offset) - 2)
			frame.pc = destination

		case 13: // PRINT_PADDR
			addr := z.packedAddress(uint32(opcode.operands[0].Value(z)), true)
			text, _ := zstring.Decode(z.Memory, addr, uint32(len(z.Memory)), z.Version(), z.Alphabets, z.AbbreviationTableBase(), false)
			z.appendText(text)

		case 14: // LOAD
			value := opcode.operands[0].Value(z)
			z.writeVariable(z.readIncPC(frame), z.readVariable(uint8(value), true), false)

		case 15: // NOT or CALL_1n
			if z.Version() < 5 {
				val := opcode.operands[0].Value(z)
				z.writeVariable(z.readIncPC(frame), ^val, false)
			} else {
				z.call(&opcode, procedure)
			}
		default:
			panic(fmt.Sprintf("Invalid 1OP opcode 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
		}

	case OP2:
		switch opcode.opcodeNumber {
		case 1: // JE
			a := opcode.operands[0].Value(z)
			branch := false
			for _, b := range opcode.operands[1:len(opcode.operands)] {
				if a == b.Value(z) {
					branch = true
				}
			}

			z.handleBranch(frame, branch)

		case 2: // JL
			a := int16(opcode.operands[0].Value(z))
			b := int16(opcode.operands[1].Value(z))

			z.handleBranch(frame, a < b)

		case 3: // JG
			a := int16(opcode.operands[0].Value(z))
			b := int16(opcode.operands[1].Value(z))

			z.handleBranch(frame, a > b)

		case 4: // DEC_CHK
			variable := uint8(opcode.operands[0].Value(z))
			newValue := int16(z.readVariable(variable, true)) - 1
			z.writeVariable(variable, uint16(newValue), true)
			branch := int16(newValue) < int16(opcode.operands[1].Value(z))

			z.handleBranch(frame, branch)

		case 5: // INC_CHK
			variable := uint8(opcode.operands[0].Value(z))
			newValue := z.readVariable(variable, true) + 1
			z.writeVariable(variable, newValue, true)
			branch := int16(newValue) > int16(opcode.operands[1].Value(z))

			z.handleBranch(frame, branch)

		case 6: // JIN
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			z.handleBranch(frame, obj.Parent == opcode.operands[1].Value(z))

		case 7: // TEST
			bitmap := opcode.operands[0].Value(z)
			flags := opcode.operands[1].Value(z)

			branch := bitmap&flags == flags
			z.handleBranch(frame, branch)

		case 8: // OR
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)|opcode.operands[1].Value(z), false)

		case 9: // AND
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)&opcode.operands[1].Value(z), false)

		case 10: // TEST_ATTR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			z.handleBranch(frame, obj.TestAttribute(opcode.operands[1].Value(z)))

		case 11: // SET_ATTR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			obj.SetAttribute(opcode.operands[1].Value(z), z.Memory, z.Version())

		case 12: // CLEAR_ATTR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			obj.ClearAttribute(opcode.operands[1].Value(z), z.Memory, z.Version())

		case 13: // STORE
			z.writeVariable(uint8(opcode.operands[0].Value(z)), opcode.operands[1].Value(z), true)

		case 14: // INSERT_OBJ
			z.MoveObject(opcode.operands[0].Value(z), opcode.operands[1].Value(z))

		case 15: // LOADW
			z.writeVariable(z.readIncPC(frame), z.readHalfWord(uint32(opcode.operands[0].Value(z)+2*opcode.operands[1].Value(z))), false)

		case 16: // LOADB
			z.writeVariable(z.readIncPC(frame), uint16(z.readByte(uint32(opcode.operands[0].Value(z)+opcode.operands[1].Value(z)))), false)

		case 17: // GET_PROP
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			prop := obj.GetProperty(uint8(opcode.operands[1].Value(z)), z.Memory, z.Version(), z.ObjectTableBase())

			value := uint16(prop.Data[0])
			if len(prop.Data) == 2 {
				value = binary.BigEndian.Uint16(prop.Data)
			} else if len(prop.Data) > 2 {
				panic("Can't get property with length > 2 using get_prop")
			}

			z.writeVariable(z.readIncPC(frame), value, false)

		case 18: // GET_PROP_ADDR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			prop := obj.GetProperty(uint8(opcode.operands[1].Value(z)), z.Memory, z.Version(), z.ObjectTableBase())
			z.writeVariable(z.readIncPC(frame), uint16(prop.DataAddress), false)

		case 19: // GET_NEXT_PROP
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
			nextProp := obj.GetNextProperty(uint8(opcode.operands[1].Value(z)), z.Memory, z.Version(), z.ObjectTableBase())
			z.writeVariable(z.readIncPC(frame), uint16(nextProp), false)

		case 20: // ADD
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)+opcode.operands[1].Value(z), false)

		case 21: // SUB
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)-opcode.operands[1].Value(z), false)

		case 22: // MUL
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)*opcode.operands[1].Value(z), false)

		case 23: // DIV
			numerator := int16(opcode.operands[0].Value(z))
			denominator := int16(opcode.operands[1].Value(z))
			if denominator == 0 {
				panic("Invalid div by zero operation")
			}
			z.writeVariable(z.readIncPC(frame), uint16(numerator/denominator), false)

		case 24: // MOD
			numerator := int16(opcode.operands[0].Value(z))
			denominator := int16(opcode.operands[1].Value(z))
			if denominator == 0 {
				panic("Invalid mod by zero operation")
			}
			z.writeVariable(z.readIncPC(frame), uint16(numerator%denominator), false)

		case 25: // call_2s
			if z.Version() < 4 {
				panic("Invalid call_2s routine on v1-3")
			}

			z.call(&opcode, function)

		case 26: // CALL_2n
			if z.Version() < 5 {
				panic("Invalid call_2s routine on v1-4")
			}

			z.call(&opcode, procedure)

		case 27: // set_colour
			if z.Version() < 5 {
				panic("Invalid set_colour routine on v1-4")
			}
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))

		case 28: // throw
			if z.Version() < 5 {
				panic("Invalid throw routine on v1-4")
			}
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))

		case 0, 29, 30, 31: // blank
			panic(fmt.Sprintf("Unused 2OP opcode number: %x", opcode.opcodeNumber))

		default:
			panic("Invalid state, interpreter bug")
		}

	case VAR:
		if opcode.opcodeForm == extForm {
			switch opcode.opcodeByte {
			case 0x00:
				panic("Save not implemented")
			case 0x01:
				panic("Restore not implemented")
			case 0x02: // LOG_SHIFT
				num := opcode.operands[0].Value(z)
				places := int16(opcode.operands[1].Value(z))
				var result uint16

				if places >= 0 {
					result = num << uint16(places)
				} else {
					result = num >> (-1 * places)
				}

				z.writeVariable(z.readIncPC(frame), result, false)
			case 0x03: // ART_SHIFT
				num := int16(opcode.operands[0].Value(z))
				places := int16(opcode.operands[1].Value(z))
				var result uint16

				if places >= 0 {
					result = uint16(num << uint16(places))
				} else {
					result = uint16(num >> (-1 * places))
				}

				z.writeVariable(z.readIncPC(frame), result, false)

			case 0x09: // SAVE_UNDO
				z.saveUndo()
				z.writeVariable(z.readIncPC(frame), uint16(1), false) // Save always succeeds in this environment

			case 0x0a: // RESTORE_UNDO
				response := z.restoreUndo()
				frame = z.callStack.peek()
				z.writeVariable(z.readIncPC(frame), response, false) // Restore always says that it's done and continues from previous save

			default:
				panic(fmt.Sprintf("EXT Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
			}
		} else {
			switch opcode.opcodeNumber {
			case 0: // CALL
				z.call(&opcode, function)

			case 1: // STOREW
				address := opcode.operands[0].Value(z) + 2*opcode.operands[1].Value(z)
				value := opcode.operands[2].Value(z)
				z.writeHalfWord(uint32(address), value)

			case 2: // STOREB
				address := opcode.operands[0].Value(z) + opcode.operands[1].Value(z)
				z.writeByte(uint32(address), uint8(opcode.operands[2].Value(z)))

			case 3: // PUT_PROP
				obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
				obj.SetProperty(uint8(opcode.operands[1].Value(z)), opcode.operands[2].Value(z), z.Memory, z.Version(), z.ObjectTableBase())

			case 4: // SREAD
				z.read(&opcode)

			case 5: // PRINT_CHAR
				chr := uint8(opcode.operands[0].Value(z))
				if chr != 0 { // CHR 0 is valid but doesn't do anything so don't pass it through
					z.appendText(string(chr))
				}

				// TODO - Should I be rejecting other characters here? Non-output ansi codes perhaps

			case 6: // PRINT_NUM
				z.appendText(strconv.Itoa(int(int16(opcode.operands[0].Value(z)))))

			case 7: // RANDOM
				n := int16(opcode.operands[0].Value(z))
				result := uint16(0)

				if n < 0 {
					z.rng.Seed(int64(n))
				} else if n == 0 {
					z.rng.Seed(time.Now().UnixNano())
				} else {
					result = uint16(z.rng.Int31n(int32(n)))
				}

				z.writeVariable(z.readIncPC(frame), result, false)
			case 8: // PUSH
				frame.push(opcode.operands[0].Value(z))

			case 9: // PULL
				z.writeVariable(uint8(opcode.operands[0].Value(z)), frame.pop(), true)

			case 10: // SPLIT_WINDOW
				if z.Version() < 3 {
					panic("Can't call SPLIT_WINDOW on pre v3 z-machine")
				}

				lines := opcode.operands[0].Value(z)
				z.screenModel.UpperWindowHeight = int(lines)

				z.outputChannel <- z.screenModel

			case 11: // SET_WINDOW
				if z.Version() < 3 {
					panic("Can't call SET_WINDOW on pre v3 z-machine")
				}
				window := opcode.operands[0].Value(z)
				z.screenModel.LowerWindowActive = window == 0
				z.outputChannel <- z.screenModel

			case 12: // CALL_VS2
				z.call(&opcode, function)

			case 15: // SET_CURSOR
				x := opcode.operands[0].Value(z)
				y := opcode.operands[1].Value(z)

				if z.Version() == 6 {
					panic("Cursors are more complex on v6")
				}

				// TODO - Pretty sure you can't set the cursor on lower window v<=5
				if !z.screenModel.LowerWindowActive {
					z.screenModel.UpperWindowCursorX = int(x)
					z.screenModel.UpperWindowCursorY = int(y)
					z.outputChannel <- z.screenModel
				}

			case 17: // SET_TEXT_STYLE
				if z.Version() >= 4 {
					mask := uint8(opcode.operands[0].Value(z))

					if z.screenModel.LowerWindowActive {
						z.screenModel.LowerWindowTextStyle = TextStyle(mask)
					} else {
						z.screenModel.UpperWindowTextStyle = TextStyle(mask)
					}

					z.outputChannel <- z.screenModel
				} else {
					panic("Can't set text style on version <=4")
				}

			case 18: // BUFFER_MODE
				// TODO - Don't think i care about this, not bothering with buffering output

			case 19: // OUTPUT_STREAM
				stream := int16(opcode.operands[0].Value(z))

				switch stream {
				case 1, -1:
					z.streams.Screen = stream > 0
				case 2, -2:
					z.streams.Transcript = stream > 0
				case 3:
					// TODO - Handle width of v6+ formatted memory stream data
					z.streams.Memory = true
					z.streams.MemoryStreamData = append(z.streams.MemoryStreamData, MemoryStreamData{
						baseAddress: uint32(opcode.operands[1].Value(z)),
						ptr:         uint32(opcode.operands[1].Value(z)) + 2, // Skip size word
					})
				case -3:
					if z.streams.Memory {
						// Store the amount of data written into the size word then close the current stream
						currentActiveStream := z.streams.MemoryStreamData[len(z.streams.MemoryStreamData)-1]
						sizeWordAddress := currentActiveStream.baseAddress
						// Note the extra -3 here is because ptr starts 2 past base address for size word and ptr always points to next unused address
						binary.BigEndian.PutUint16(z.Memory[sizeWordAddress:sizeWordAddress+2], uint16(currentActiveStream.ptr-currentActiveStream.baseAddress-2))

						// Note that there might be historical streams still active, these act as a stack
						z.streams.MemoryStreamData = z.streams.MemoryStreamData[:len(z.streams.MemoryStreamData)-1]
						if len(z.streams.MemoryStreamData) == 0 {
							z.streams.Memory = false
						}
					}
				case 4, -4:
					z.streams.CommandScript = stream > 0
				}

			case 23: // SCAN_TABLE
				test := opcode.operands[0].Value(z)
				tableAddress := opcode.operands[1].Value(z)
				length := opcode.operands[2].Value(z)
				form := uint16(0x82)

				if len(opcode.operands) == 4 {
					form = opcode.operands[3].Value(z)
				}

				result := ztable.ScanTable(z.Memory, test, uint32(tableAddress), length, form)

				z.writeVariable(z.readIncPC(frame), uint16(result), false)

				z.handleBranch(frame, result != 0)

			case 24: // NOT
				val := opcode.operands[0].Value(z)
				z.writeVariable(z.readIncPC(frame), ^val, false)

			case 25: // CALL_VN
				z.call(&opcode, procedure)

			case 26: // CALL_VN2
				z.call(&opcode, procedure)

			case 27: // TOKENISE
				text := opcode.operands[0].Value(z)
				parseBuffer := opcode.operands[1].Value(z)
				dictionaryToUse := z.dictionary
				flag := false

				if len(opcode.operands) > 2 {
					dictionaryAddress := opcode.operands[2].Value(z)

					// TODO - Handle special case custom dictionaries with negative number of entries (unsorted)
					dictionaryToUse = dictionary.ParseDictionary(z.Memory, uint32(dictionaryAddress), z.Version(), z.Alphabets, z.AbbreviationTableBase())

					if len(opcode.operands) == 4 {
						flag = opcode.operands[3].Value(z) != 0

						panic("TODO - Haven't really implemented this yet so crash if a story actually uses it")
					}
				}

				z.Tokenise(uint32(text), uint32(parseBuffer), dictionaryToUse, flag)

			case 29: // COPY_TABLE
				ztable.CopyTable(z.Memory, opcode.operands[0].Value(z), opcode.operands[1].Value(z), int16(opcode.operands[2].Value(z)))

			case 30: // PRINT_TABLE
				addr := opcode.operands[0].Value(z)
				width := opcode.operands[1].Value(z)
				height := uint16(1)
				skip := uint16(0)

				if len(opcode.operands) > 2 {
					height = opcode.operands[2].Value(z)

					if len(opcode.operands) > 3 {
						skip = opcode.operands[3].Value(z)
					}
				}
				z.appendText(ztable.PrintTable(z.Memory, uint32(addr), width, height, skip))

			case 31: // CHECK_ARG_COUNT
				arg := opcode.operands[0].Value(z)
				branch := arg <= uint16(frame.numValuesPassed)

				z.handleBranch(frame, branch)

			default:
				panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
			}
		}
	}

	return true
}
