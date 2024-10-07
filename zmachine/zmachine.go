package zmachine

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/davetcode/goz/dictionary"
	"github.com/davetcode/goz/zstring"
)

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
	memory             []uint8
	dictionary         *dictionary.Dictionary
	rng                rand.Rand
	textOutputChannel  chan<- string
	stateChangeChannel chan<- StateChangeRequest
	inputChannel       <-chan string
}

func (z *ZMachine) version() uint8           { return z.memory[0] }
func (z *ZMachine) flagByte1() uint8         { return z.memory[0x01] }
func (z *ZMachine) releaseNumber() uint16    { return binary.BigEndian.Uint16(z.memory[0x02:0x04]) }
func (z *ZMachine) pagedMemoryBase() uint16  { return binary.BigEndian.Uint16(z.memory[0x04:0x06]) }
func (z *ZMachine) firstInstruction() uint16 { return binary.BigEndian.Uint16(z.memory[0x06:0x08]) }
func (z *ZMachine) dictionaryBase() uint16   { return binary.BigEndian.Uint16(z.memory[0x08:0x0a]) }
func (z *ZMachine) objectTableBase() uint16  { return binary.BigEndian.Uint16(z.memory[0x0a:0x0c]) }
func (z *ZMachine) globalVariableBase() uint16 {
	return binary.BigEndian.Uint16(z.memory[0x0c:0x0e])
}
func (z *ZMachine) staticMemoryBase() uint16 { return binary.BigEndian.Uint16(z.memory[0x0e:0x10]) }
func (z *ZMachine) flagByte2() uint8         { return z.memory[0x10] }
func (z *ZMachine) flagByte3() uint8         { return z.memory[0x11] }
func (z *ZMachine) serialCode() []uint8      { return z.memory[0x12:0x18] }
func (z *ZMachine) abbreviationTableBase() uint16 {
	return binary.BigEndian.Uint16(z.memory[0x18:0x1a])
}
func (z *ZMachine) fileLengthDiv() uint16               { return binary.BigEndian.Uint16(z.memory[0x1a:0x1c]) }
func (z *ZMachine) fileChecksum() uint16                { return binary.BigEndian.Uint16(z.memory[0x1c:0x1e]) }
func (z *ZMachine) interpreterNumber() uint8            { return z.memory[0x1e] }
func (z *ZMachine) interpreterVersion() uint8           { return z.memory[0x1f] }
func (z *ZMachine) screenHeightLines() uint8            { return z.memory[0x20] }
func (z *ZMachine) screenWidthChars() uint8             { return z.memory[0x21] }
func (z *ZMachine) screenWidthUnits() uint16            { return binary.BigEndian.Uint16(z.memory[0x22:0x24]) }
func (z *ZMachine) screenHeightUnits() uint16           { return binary.BigEndian.Uint16(z.memory[0x24:0x26]) }
func (z *ZMachine) fontHeight() uint8                   { return z.memory[0x26] }
func (z *ZMachine) fontWidth() uint8                    { return z.memory[0x27] }
func (z *ZMachine) routinesOffset() uint16              { return binary.BigEndian.Uint16(z.memory[0x28:0x2a]) }
func (z *ZMachine) stringOffset() uint16                { return binary.BigEndian.Uint16(z.memory[0x2a:0x2c]) }
func (z *ZMachine) defaultBackgroundColorNumber() uint8 { return z.memory[0x2c] }
func (z *ZMachine) defaultForegroundColorNumber() uint8 { return z.memory[0x2d] }
func (z *ZMachine) terminatingCharTableBase() uint16 {
	return binary.BigEndian.Uint16(z.memory[0x2e:0x30])
}
func (z *ZMachine) outputStream3Width() uint16 {
	return binary.BigEndian.Uint16(z.memory[0x30:0x32])
}
func (z *ZMachine) standardRevisionNumber() uint16 {
	return binary.BigEndian.Uint16(z.memory[0x32:0x34])
}
func (z *ZMachine) alternativeCharSetBaseAddress() uint16 {
	return binary.BigEndian.Uint16(z.memory[0x34:0x36])
}
func (z *ZMachine) extensionTableBaseAddress() uint16 {
	return binary.BigEndian.Uint16(z.memory[0x36:0x38])
}
func (z *ZMachine) playerLoginName() []uint8 { return z.memory[0x38:0x40] }

func (z *ZMachine) packedAddress(originalAddress uint32, isZString bool) uint32 {
	switch {
	case z.version() < 4:
		return 2 * originalAddress
	case z.version() < 6:
		return 4 * originalAddress
	case z.version() < 8:
		offset := z.routinesOffset()
		if isZString {
			offset = z.stringOffset()
		}
		return 4*originalAddress + 8*uint32(offset)
	case z.version() == 8:
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
	return z.memory[address]
}

func (z *ZMachine) writeByte(address uint32, value uint8) {
	// TODO - Lots of the memory is read only, need to add validation here
	z.memory[address] = value
}

func (z *ZMachine) readHalfWord(address uint32) uint16 {
	return binary.BigEndian.Uint16(z.memory[address : address+2])
}

func (z *ZMachine) writeHalfWord(address uint32, value uint16) {
	// TODO - Lots of the memory is read only, need to add validation here
	binary.BigEndian.PutUint16(z.memory[address:address+2], value)
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

func LoadRom(rom []uint8, inputChannel <-chan string, textOutputChannel chan<- string, stateChangeChannel chan<- StateChangeRequest) *ZMachine {
	machine := ZMachine{
		memory:             rom,
		inputChannel:       inputChannel,
		textOutputChannel:  textOutputChannel,
		stateChangeChannel: stateChangeChannel,
	}

	// TODO - Is the dictionary static? If not shouldn't cache like this
	machine.dictionary = dictionary.ParseDictionary(machine.memory[machine.dictionaryBase():], uint32(machine.dictionaryBase()), machine.version())

	// V6+ uses a packed address and a routine for the initial function
	if machine.version() >= 6 {
		packedAddress := machine.packedAddress(uint32(machine.firstInstruction()), false)

		machine.callStack.push(CallStackFrame{
			pc:     packedAddress + 1,
			locals: make([]uint16, machine.memory[packedAddress]),
		})
	} else {
		machine.callStack.push(CallStackFrame{
			pc:     uint32(machine.firstInstruction()),
			locals: make([]uint16, 0),
		})
	}

	return &machine
}

func (z *ZMachine) call(opcode *Opcode) {
	routineAddress := z.packedAddress(uint32(opcode.operands[0].Value(z)), false)

	// Special case, if routine address is 0 then no call is made and 0 is stored in the return address
	if routineAddress == 0 {
		z.writeVariable(z.readIncPC(z.callStack.peek()), 0)
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
			if z.version() < 5 {
				locals[i] = z.readHalfWord(routineAddress)
			}
		}

		if z.version() < 5 {
			routineAddress += 2
		}
	}

	z.callStack.push(CallStackFrame{
		pc:              routineAddress,
		locals:          locals,
		routineStack:    make([]uint16, 0),
		routineType:     procedure, // TODO - Not really sure what this is, v3+ only
		numValuesPassed: len(opcode.operands),
		framePointer:    0, // TODO - Only used for try/catch in later versions
	})
}

func (z *ZMachine) handleBranch(frame *CallStackFrame, result bool) {
	branchArg1 := z.readIncPC(frame)

	branchReversed := (branchArg1>>7)&1 == 0
	singleByte := (branchArg1>>6)&1 == 1
	offset := int32(branchArg1 & 0b11_1111)

	if !singleByte {
		sign := int32(1)
		if (branchArg1>>5)&1 == 1 {
			sign = -1
		}
		offset = sign*int32((branchArg1&0b11_1111)) | int32(z.readIncPC(frame))
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

func tokeniseSingleWord(bytes []uint8, wordStartPtr uint32, dictionary *dictionary.Dictionary) word {
	// TODO - Description here suggests that the string should be turned back into a z-string for comparison with the dictionary although this might work anyway
	str := string(bytes)

	dictionaryAddress := dictionary.Find(str)

	return word{
		bytes:             bytes,
		startingLocation:  wordStartPtr,
		dictionaryAddress: dictionaryAddress,
	}
}

func (z *ZMachine) Tokenise(baddr1 uint32, baddr2 uint32) {
	bytesRead := 0
	words := make([]word, 0)
	startingLocation := baddr1
	currentLocation := startingLocation

	for _, chr := range z.memory[baddr1:] {
		if z.version() < 5 {
			if chr == 0 {
				words = append(words, tokeniseSingleWord(z.memory[startingLocation:currentLocation], startingLocation, z.dictionary))
				break
			}

			if chr == ' ' { // space is always a separator
				words = append(words, tokeniseSingleWord(z.memory[startingLocation:currentLocation], startingLocation, z.dictionary))
				startingLocation = currentLocation
			} else {
				for _, separator := range z.dictionary.Header.InputCodes {
					if chr == separator {
						words = append(words, tokeniseSingleWord(z.memory[startingLocation:currentLocation], startingLocation, z.dictionary))
						words = append(words, tokeniseSingleWord(z.memory[currentLocation:currentLocation+1], startingLocation, z.dictionary))
						startingLocation = currentLocation + 1
					}
				}
			}
		} else {
			panic("TODO - Handle tokenise on v5+")
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
	z.callStack.pop()
	newFrame := z.callStack.peek()

	destination := z.readIncPC(newFrame)
	z.writeVariable(destination, val)
}

func (z *ZMachine) appendText(s string) {
	z.textOutputChannel <- s
}

func (z *ZMachine) StepMachine() {
	if z.callStack.peek().pc == 0x5f0f {
		z.memory[0] = z.memory[0]
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
			text, bytesRead := zstring.ReadZString(z.memory[frame.pc:], z.version())
			frame.pc += bytesRead
			z.appendText(text)

		case 3: // PRINT_RET
			text, bytesRead := zstring.ReadZString(z.memory[frame.pc:], z.version())
			frame.pc += bytesRead
			z.appendText(text)
			z.appendText("\n")
			z.retValue(1)

		case 8: // RET_POPPED
			v := frame.pop()
			z.retValue(v)

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
			sibling := z.getObject(opcode.operands[0].Value(z)).sibling
			z.writeVariable(z.readIncPC(frame), sibling)

			z.handleBranch(frame, sibling != 0)

		case 2: // GET_CHILD
			child := z.getObject(opcode.operands[0].Value(z)).child
			z.writeVariable(z.readIncPC(frame), child)

			z.handleBranch(frame, child != 0)

		case 3: // GET_PARENT
			z.writeVariable(z.readIncPC(frame), z.getObject(opcode.operands[0].Value(z)).parent)

		case 4: // GET_PROP_LEN
			property := z.getPropertyByAddress(uint32(opcode.operands[0].Value(z) - 1))
			z.writeVariable(z.readIncPC(frame), uint16(property.length))

		case 5: // INC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable)+1)

		case 6: // DEC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable)-1)

		case 10: // PRINT_OBJ
			z.appendText(z.getObjectName(opcode.operands[0].Value(z)))

		case 11: // RET
			v := opcode.operands[0].Value(z)
			z.retValue(v)

		case 12: // JUMP
			offset := int16(opcode.operands[0].Value(z))
			destination := uint32(int16(frame.pc) + offset - 2)
			frame.pc = destination

		case 13: // PRINT_PADDR
			addr := z.packedAddress(uint32(opcode.operands[0].Value(z)), true)
			text, _ := zstring.ReadZString(z.memory[addr:], z.version())
			z.appendText(text)

		case 14: // LOAD
			value := opcode.operands[0].Value(z)
			z.writeVariable(z.readIncPC(frame), z.readVariable(uint8(value)))
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
			branch := z.readVariable(variable) < opcode.operands[1].Value(z)

			z.handleBranch(frame, branch)

		case 5: // INC_CHK
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable)+1)
			branch := z.readVariable(variable) > opcode.operands[1].Value(z)

			z.handleBranch(frame, branch)

		case 6: // JIN
			obj := z.getObject(opcode.operands[0].Value(z))
			z.handleBranch(frame, obj.parent == opcode.operands[1].Value(z))

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
			obj := z.getObject(opcode.operands[0].Value(z))
			attr := 63 - opcode.operands[1].Value(z)
			mask := uint64(1) << attr
			branch := obj.attributes&mask == mask
			z.handleBranch(frame, branch)

		case 11: // SET_ATTR
			obj := z.getObject(opcode.operands[0].Value(z))
			attr := 63 - opcode.operands[1].Value(z)
			mask := uint64(1) << attr
			obj.attributes |= mask

		case 12: // CLEAR_ATTR
			obj := z.getObject(opcode.operands[0].Value(z))
			attr := 63 - opcode.operands[1].Value(z)
			mask := uint64(1) << attr
			obj.attributes &= ^mask

		case 13: // STORE
			z.writeVariable(uint8(opcode.operands[0].Value(z)), opcode.operands[1].Value(z))

		case 14: // INSERT_OBJ
			z.moveObject(opcode.operands[0].Value(z), opcode.operands[1].Value(z))

		case 15: // LOADW
			z.writeVariable(z.readIncPC(frame), z.readHalfWord(uint32(opcode.operands[0].Value(z)+2*opcode.operands[1].Value(z))))

		case 16: // LOADB
			z.writeVariable(z.readIncPC(frame), uint16(z.readByte(uint32(opcode.operands[0].Value(z)+opcode.operands[1].Value(z)))))

		case 17: // GET_PROP
			prop := z.getObjectProperty(opcode.operands[0].Value(z), uint8(opcode.operands[1].Value(z)))

			value := uint16(prop.data[0])
			if len(prop.data) == 2 {
				value = binary.BigEndian.Uint16(prop.data)
			} else if len(prop.data) > 2 {
				panic("Can't get property with length > 2 using get_prop")
			}

			z.writeVariable(z.readIncPC(frame), value)

		case 18: // GET_PROP_ADDR
			prop := z.getObjectProperty(opcode.operands[0].Value(z), uint8(opcode.operands[1].Value(z)))
			z.writeVariable(z.readIncPC(frame), prop.address+1)

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
			if z.version() < 4 {
				panic("Invalid call_2s routine on v1-3")
			}
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))

		case 26: // call_2n
			if z.version() < 5 {
				panic("Invalid call_2s routine on v1-4")
			}
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))

		case 27: // set_colour
			if z.version() < 5 {
				panic("Invalid set_colour routine on v1-4")
			}
			panic(fmt.Sprintf("Opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc))

		case 28: // throw
			if z.version() < 5 {
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
			z.call(&opcode)

		case 1: // STOREW
			address := opcode.operands[0].Value(z) + 2*opcode.operands[1].Value(z)
			value := opcode.operands[2].Value(z)
			z.writeHalfWord(uint32(address), value)

		case 2: // STOREB
			address := opcode.operands[0].Value(z) + opcode.operands[1].Value(z)
			z.writeByte(uint32(address), uint8(opcode.operands[2].Value(z)))

		case 3: // PUT_PROP
			z.setObjectProperty(opcode.operands[0].Value(z), uint8(opcode.operands[1].Value(z)), opcode.operands[2].Value(z))

		case 4: // SREAD
			// TODO - Somehow let UI know how many chars to accept
			z.stateChangeChannel <- WaitForInput
			rawText := <-z.inputChannel
			textBufferPtr := opcode.operands[0].Value(z)
			parseBufferPtr := opcode.operands[1].Value(z)

			bufferSize := z.readByte(uint32(textBufferPtr))
			rawTextBytes := []byte(strings.ToLower(rawText))
			for ix, chr := range rawTextBytes {
				if ix > int(bufferSize) { // TODO - Not 100% sure on whether this is >= or some other off by one value. Docs are unclear
					break // Too many characters provided
				}

				if (chr >= 32 && chr <= 126) || (chr >= 155 && chr <= 251) {
					z.writeByte(uint32(textBufferPtr+uint16(ix)), chr)
				} else {
					z.writeByte(uint32(textBufferPtr+uint16(ix)), 32)
				}
			}

			z.Tokenise(uint32(textBufferPtr), uint32(parseBufferPtr))

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
