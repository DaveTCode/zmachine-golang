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
)

type StatusBar struct {
	PlaceName string
	Score     int
	Moves     int
}

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

type CallStackFrame struct {
	pc              uint32   // TODO - What is the usual limit to this number?
	routineStack    []uint16 // TODO - Really a stack, check how it's used to see if we care
	locals          []uint16
	routineType     RoutineType // v3+ only
	numValuesPassed int         // v5+ only
	framePointer    uint32
}

func (f *CallStackFrame) push(i uint16) {
	f.routineStack = append(f.routineStack, i)
}

func (f *CallStackFrame) pop() uint16 {
	i := f.routineStack[len(f.routineStack)-1]
	f.routineStack = f.routineStack[:len(f.routineStack)-1]
	return i
}

type CallStack struct {
	frames []CallStackFrame
}

func (s *CallStack) push(frame CallStackFrame) {
	s.frames = append(s.frames, frame)
}

func (s *CallStack) pop() CallStackFrame {
	stackSize := len(s.frames)
	frame := s.frames[stackSize-1]
	s.frames = s.frames[:stackSize-1]

	return frame
}

func (s *CallStack) peek() *CallStackFrame {
	return &s.frames[len(s.frames)-1]
}

type ZMachine struct {
	callStack          CallStack
	Memory             []uint8
	dictionary         *dictionary.Dictionary
	rng                rand.Rand
	Alphabets          *zstring.Alphabets
	textOutputChannel  chan<- string
	stateChangeChannel chan<- StateChangeRequest
	inputChannel       <-chan string
	statusBarChannel   chan<- StatusBar
}

func (z *ZMachine) Version() uint8           { return z.Memory[0] }
func (z *ZMachine) flagByte1() uint8         { return z.Memory[0x01] }
func (z *ZMachine) releaseNumber() uint16    { return binary.BigEndian.Uint16(z.Memory[0x02:0x04]) }
func (z *ZMachine) pagedMemoryBase() uint16  { return binary.BigEndian.Uint16(z.Memory[0x04:0x06]) }
func (z *ZMachine) firstInstruction() uint16 { return binary.BigEndian.Uint16(z.Memory[0x06:0x08]) }
func (z *ZMachine) dictionaryBase() uint16   { return binary.BigEndian.Uint16(z.Memory[0x08:0x0a]) }
func (z *ZMachine) ObjectTableBase() uint16  { return binary.BigEndian.Uint16(z.Memory[0x0a:0x0c]) }
func (z *ZMachine) globalVariableBase() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x0c:0x0e])
}
func (z *ZMachine) staticMemoryBase() uint16 { return binary.BigEndian.Uint16(z.Memory[0x0e:0x10]) }
func (z *ZMachine) flagByte2() uint8         { return z.Memory[0x10] }
func (z *ZMachine) flagByte3() uint8         { return z.Memory[0x11] }
func (z *ZMachine) serialCode() []uint8      { return z.Memory[0x12:0x18] }
func (z *ZMachine) abbreviationTableBase() uint16 {
	return binary.BigEndian.Uint16(z.Memory[0x18:0x1a])
}
func (z *ZMachine) fileLengthDiv() uint16               { return binary.BigEndian.Uint16(z.Memory[0x1a:0x1c]) }
func (z *ZMachine) fileChecksum() uint16                { return binary.BigEndian.Uint16(z.Memory[0x1c:0x1e]) }
func (z *ZMachine) interpreterNumber() uint8            { return z.Memory[0x1e] }
func (z *ZMachine) interpreterVersion() uint8           { return z.Memory[0x1f] }
func (z *ZMachine) screenHeightLines() uint8            { return z.Memory[0x20] }
func (z *ZMachine) screenWidthChars() uint8             { return z.Memory[0x21] }
func (z *ZMachine) screenWidthUnits() uint16            { return binary.BigEndian.Uint16(z.Memory[0x22:0x24]) }
func (z *ZMachine) screenHeightUnits() uint16           { return binary.BigEndian.Uint16(z.Memory[0x24:0x26]) }
func (z *ZMachine) fontHeight() uint8                   { return z.Memory[0x26] }
func (z *ZMachine) fontWidth() uint8                    { return z.Memory[0x27] }
func (z *ZMachine) routinesOffset() uint16              { return binary.BigEndian.Uint16(z.Memory[0x28:0x2a]) }
func (z *ZMachine) stringOffset() uint16                { return binary.BigEndian.Uint16(z.Memory[0x2a:0x2c]) }
func (z *ZMachine) defaultBackgroundColorNumber() uint8 { return z.Memory[0x2c] }
func (z *ZMachine) defaultForegroundColorNumber() uint8 { return z.Memory[0x2d] }
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
	v := z.readByte(uint32(frame.pc))
	frame.pc++
	return v
}

func (z *ZMachine) readHalfWordIncPC(frame *CallStackFrame) uint16 {
	v := z.readHalfWord(uint32(frame.pc))
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

func (z *ZMachine) readVariable(variable uint8) uint16 {
	currentCallFrame := z.callStack.peek()

	switch {
	case variable == 0: // Magic stack variable
		if len(currentCallFrame.routineStack) == 0 {
			panic("Attempt to read from empty routine stack")
		}

		return currentCallFrame.pop()
	case variable < 16: // Routine local variables

		if variable-1 >= uint8(len(currentCallFrame.locals)) {
			panic("Attempt to access non-existing local variable")
		}

		return currentCallFrame.locals[variable-1]
	default: // Global variables
		return z.readHalfWord(uint32(z.globalVariableBase() + 2*(uint16(variable)-16)))
	}
}

func (z *ZMachine) writeVariable(variable uint8, value uint16) {
	currentCallFrame := z.callStack.peek()

	switch {
	case variable == 0: // Magic stack variable
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

func LoadRom(rom []uint8, inputChannel <-chan string, textOutputChannel chan<- string, stateChangeChannel chan<- StateChangeRequest, statusBarChannel chan<- StatusBar) *ZMachine {
	machine := ZMachine{
		Memory:             rom,
		inputChannel:       inputChannel,
		textOutputChannel:  textOutputChannel,
		stateChangeChannel: stateChangeChannel,
		statusBarChannel:   statusBarChannel,
	}

	// Load custom alphabets on v5+
	machine.Alphabets = zstring.LoadAlphabets(machine.Version(), rom, machine.alternativeCharSetBaseAddress())

	// TODO - Is the dictionary static? If not shouldn't cache like this
	machine.dictionary = dictionary.ParseDictionary(machine.Memory[machine.dictionaryBase():], uint32(machine.dictionaryBase()), machine.Version(), machine.Alphabets)

	// V6+ uses a packed address and a routine for the initial function
	if machine.Version() >= 6 {
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
			z.writeVariable(z.readIncPC(z.callStack.peek()), 0)
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

func (z *ZMachine) Tokenise(baddr1 uint32, baddr2 uint32) {
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
			words = append(words, tokeniseSingleWord(z.Memory[startingLocation:currentLocation], startingLocation, z.dictionary, z.Version(), z.Alphabets))
			break
		}

		if chr == ' ' { // space is always a separator
			words = append(words, tokeniseSingleWord(z.Memory[startingLocation:currentLocation], startingLocation, z.dictionary, z.Version(), z.Alphabets))
			startingLocation = currentLocation + 1
		} else {
			for _, separator := range z.dictionary.Header.InputCodes {
				if chr == separator {
					words = append(words, tokeniseSingleWord(z.Memory[startingLocation:currentLocation], startingLocation, z.dictionary, z.Version(), z.Alphabets))
					words = append(words, tokeniseSingleWord(z.Memory[currentLocation:currentLocation+1], startingLocation, z.dictionary, z.Version(), z.Alphabets))
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
		z.writeVariable(destination, val)
	}
}

func (z *ZMachine) moveObject(objId uint16, newParent uint16) {
	object := zobject.GetObject(objId, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
	destinationObject := zobject.GetObject(newParent, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
	oldParent := zobject.GetObject(object.Parent, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)

	// Remove from old location in the sibling chain
	if oldParent.Child == object.Id {
		// First child case
		oldParent.SetChild(object.Sibling, z.Version(), z.Memory)
	} else {
		// Non-first child case
		currObjId := uint16(oldParent.Child)
		for {
			if currObjId == 0 {
				break
			}

			currObj := zobject.GetObject(currObjId, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			if currObj.Sibling == object.Id {
				currObj.SetSibling(object.Sibling, z.Version(), z.Memory)
				break
			}
		}
	}

	// Set new location in the tree
	object.SetSibling(destinationObject.Child, z.Version(), z.Memory)
	object.SetParent(destinationObject.Id, z.Version(), z.Memory)
	destinationObject.SetChild(object.Id, z.Version(), z.Memory)
}

func (z *ZMachine) appendText(s string) {
	z.textOutputChannel <- s
}

func (z *ZMachine) read(opcode *Opcode) {
	if z.Version() <= 3 { // TODO - Not really sure if this is true
		currentLocation := zobject.GetObject(z.readVariable(16), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
		z.statusBarChannel <- StatusBar{
			PlaceName: currentLocation.Name,
			Score:     int(z.readVariable(17)),
			Moves:     int(z.readVariable(18)),
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
	z.stateChangeChannel <- WaitForInput
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
		z.Tokenise(uint32(opcode.operands[0].Value(z)), uint32(parseBufferPtr))
	}

	if z.Version() >= 5 {
		z.writeVariable(z.readIncPC(z.callStack.peek()), 13) // TODO - Should be the typed terminating char
	}
}

var pcHistory = make([]uint32, 100)
var pcHistoryPtr = 0

func (z *ZMachine) StepMachine() {
	pcHistory[pcHistoryPtr] = z.callStack.peek().pc
	pcHistoryPtr = (pcHistoryPtr + 1) % 100

	if z.callStack.peek().pc == 0x1189 {
		pcHistoryPtr = pcHistoryPtr + 1 - 1
	}

	opcode := ParseOpcode(z)
	frame := z.callStack.peek()

	switch opcode.operandCount {
	case OP0:
		switch opcode.opcodeNumber {
		case 0: // RTRUE
			z.retValue(1)

		case 1: // RFALSE
			z.retValue(0)

		case 2: // PRINT
			text, bytesRead := zstring.Decode(z.Memory[frame.pc:], z.Version(), z.Alphabets)
			frame.pc += bytesRead
			z.appendText(text)

		case 3: // PRINT_RET
			text, bytesRead := zstring.Decode(z.Memory[frame.pc:], z.Version(), z.Alphabets)
			frame.pc += bytesRead
			z.appendText(text)
			z.appendText("\n")
			z.retValue(1)

		case 8: // RET_POPPED
			v := frame.pop()
			z.retValue(v)

		case 10: // QUIT
			panic("TODO - Quit properly by passing information back to the calling function and tea")

		case 11: // NEWLINE
			z.appendText("\n")

		default:
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
		}
	case OP1:
		switch opcode.opcodeNumber {
		case 0: // JZ
			z.handleBranch(frame, opcode.operands[0].Value(z) == 0)

		case 1: // GET_SIBLING
			sibling := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets).Sibling
			z.writeVariable(z.readIncPC(frame), sibling)

			z.handleBranch(frame, sibling != 0)

		case 2: // GET_CHILD
			child := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets).Child
			z.writeVariable(z.readIncPC(frame), child)

			z.handleBranch(frame, child != 0)

		case 3: // GET_PARENT
			z.writeVariable(z.readIncPC(frame), zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets).Parent)

		case 4: // GET_PROP_LEN
			addr := opcode.operands[0].Value(z)
			z.writeVariable(z.readIncPC(frame), zobject.GetPropertyLength(z.Memory, uint32(addr), z.Version()))

		case 5: // INC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable)+1)

		case 6: // DEC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable)-1)

		case 7: // PRINT_ADDR
			address := opcode.operands[0].Value(z)
			str, _ := zstring.Decode(z.Memory[address:], z.Version(), z.Alphabets)
			z.appendText(str)

		case 8: // CALL_1S
			z.call(&opcode, function)

		case 10: // PRINT_OBJ
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			z.appendText(obj.Name)

		case 11: // RET
			v := opcode.operands[0].Value(z)
			z.retValue(v)

		case 12: // JUMP
			offset := int16(opcode.operands[0].Value(z))
			destination := uint32(int16(frame.pc) + offset - 2)
			frame.pc = destination

		case 13: // PRINT_PADDR
			addr := z.packedAddress(uint32(opcode.operands[0].Value(z)), true)
			text, _ := zstring.Decode(z.Memory[addr:], z.Version(), z.Alphabets)
			z.appendText(text)

		case 14: // LOAD
			value := opcode.operands[0].Value(z)
			z.writeVariable(z.readIncPC(frame), z.readVariable(uint8(value)))

		case 15: // NOT or CALL_1n
			if z.Version() < 5 {
				val := opcode.operands[0].Value(z)
				z.writeVariable(z.readIncPC(frame), ^val)
			} else {
				z.call(&opcode, procedure)
			}
		default:
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
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
			z.writeVariable(variable, z.readVariable(variable)-1)
			branch := int16(z.readVariable(variable)) < int16(opcode.operands[1].Value(z))

			z.handleBranch(frame, branch)

		case 5: // INC_CHK
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable)+1)
			branch := int16(z.readVariable(variable)) > int16(opcode.operands[1].Value(z))

			z.handleBranch(frame, branch)

		case 6: // JIN
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			z.handleBranch(frame, obj.Parent == opcode.operands[1].Value(z))

		case 7: // TEST
			bitmap := opcode.operands[0].Value(z)
			flags := opcode.operands[1].Value(z)

			branch := bitmap&flags == flags
			z.handleBranch(frame, branch)

		case 8: // OR
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)|opcode.operands[1].Value(z))

		case 9: // AND
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)&opcode.operands[1].Value(z))

		case 10: // TEST_ATTR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			z.handleBranch(frame, obj.TestAttribute(opcode.operands[1].Value(z)))

		case 11: // SET_ATTR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			obj.SetAttribute(opcode.operands[1].Value(z), z.Memory, z.Version())

		case 12: // CLEAR_ATTR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			obj.ClearAttribute(opcode.operands[1].Value(z), z.Memory, z.Version())

		case 13: // STORE
			z.writeVariable(uint8(opcode.operands[0].Value(z)), opcode.operands[1].Value(z))

		case 14: // INSERT_OBJ
			z.moveObject(opcode.operands[0].Value(z), opcode.operands[1].Value(z))

		case 15: // LOADW
			z.writeVariable(z.readIncPC(frame), z.readHalfWord(uint32(opcode.operands[0].Value(z)+2*opcode.operands[1].Value(z))))

		case 16: // LOADB
			z.writeVariable(z.readIncPC(frame), uint16(z.readByte(uint32(opcode.operands[0].Value(z)+opcode.operands[1].Value(z)))))

		case 17: // GET_PROP
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			prop := obj.GetProperty(uint8(opcode.operands[1].Value(z)), z.Memory, z.Version(), z.ObjectTableBase())

			value := uint16(prop.Data[0])
			if len(prop.Data) == 2 {
				value = binary.BigEndian.Uint16(prop.Data)
			} else if len(prop.Data) > 2 {
				panic("Can't get property with length > 2 using get_prop")
			}

			z.writeVariable(z.readIncPC(frame), value)

		case 18: // GET_PROP_ADDR
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			prop := obj.GetProperty(uint8(opcode.operands[1].Value(z)), z.Memory, z.Version(), z.ObjectTableBase())
			z.writeVariable(z.readIncPC(frame), uint16(prop.DataAddress))

		case 19:
			// TODO - get_next_prop
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))

		case 20: // ADD
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)+opcode.operands[1].Value(z))

		case 21: // SUB
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)-opcode.operands[1].Value(z))

		case 22: // MUL
			z.writeVariable(z.readIncPC(frame), opcode.operands[0].Value(z)*opcode.operands[1].Value(z))

		case 23: // DIV
			numerator := opcode.operands[0].Value(z)
			denominator := opcode.operands[1].Value(z)
			if denominator == 0 {
				panic("Invalid div by zero operation")
			}
			z.writeVariable(z.readIncPC(frame), numerator/denominator)

		case 24: // MOD
			numerator := opcode.operands[0].Value(z)
			denominator := opcode.operands[1].Value(z)
			if denominator == 0 {
				panic("Invalid mod by zero operation")
			}
			z.writeVariable(z.readIncPC(frame), numerator%denominator)

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
			obj := zobject.GetObject(opcode.operands[0].Value(z), z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets)
			obj.SetProperty(uint8(opcode.operands[1].Value(z)), opcode.operands[2].Value(z), z.Memory, z.Version(), z.ObjectTableBase())

		case 4: // SREAD
			z.read(&opcode)

		case 5: // PRINT_CHAR
			z.appendText(string(uint8(opcode.operands[0].Value(z))))

		case 6: // PRINT_NUM
			z.appendText(strconv.Itoa(int(int16(opcode.operands[0].Value(z)))))

		case 7: // RANDOM
			n := int16(opcode.operands[0].Value(z))
			result := uint16(0)

			if n < 0 {
				z.rng.Seed(int64(n))
			} else if n == 0 {
				z.rng.Seed(time.Now().UTC().UnixNano())
			} else {
				result = uint16(rand.Int31n(int32(n)))
			}

			z.writeVariable(z.readIncPC(frame), result)
		case 8: // PUSH
			frame.push(opcode.operands[0].Value(z))

		case 9: // PULL
			z.writeVariable(uint8(opcode.operands[0].Value(z)), frame.pop())

		case 17: // SET_TEXT_STYLE
			if z.Version() >= 4 {
				// TODO - Handle set_text_style
			} else {
				panic("Can't set text style on version <=4")
			}

		case 25: // CALL_VN
			z.call(&opcode, procedure)

		case 31: // CHECK_ARG_COUNT
			arg := opcode.operands[0].Value(z)
			branch := arg <= uint16(frame.numValuesPassed)

			z.handleBranch(frame, branch)

		default:
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
		}

	case EXT:
		switch opcode.opcodeNumber {
		default:
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))
		}
	}

}
