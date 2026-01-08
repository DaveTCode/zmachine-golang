package zmachine

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/davetcode/goz/dictionary"
	"github.com/davetcode/goz/zcore"
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

type Restart bool

type RuntimeError string

type Warning string

type EraseWindowRequest int

type EraseLineRequest int

type StateChangeRequest int

const (
	WaitForInput     StateChangeRequest = iota
	WaitForCharacter StateChangeRequest = iota
	Running          StateChangeRequest = iota
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
	callStack            CallStack
	Core                 zcore.Core
	dictionary           *dictionary.Dictionary
	screenModel          ScreenModel
	streams              Streams
	rng                  rand.Rand
	Alphabets            *zstring.Alphabets
	outputChannel        chan<- any
	inputChannel         <-chan string
	UndoStates           InMemorySaveStateCache
	nextFramePointer     uint16          // Used for catch/throw in V5+
	issuedWarnings       map[string]bool // Track warnings to implement "will ignore further occurrences"
	currentInstructionPC uint32          // PC of the current instruction (for warnings)
}

func (z *ZMachine) packedAddress(originalAddress uint32, isZString bool) uint32 {
	switch {
	case z.Core.Version < 4:
		return 2 * originalAddress
	case z.Core.Version < 6:
		return 4 * originalAddress
	case z.Core.Version < 8:
		offset := z.Core.RoutinesOffset
		if isZString {
			offset = z.Core.StringOffset
		}
		return 4*originalAddress + 8*uint32(offset)
	case z.Core.Version == 8:
		return 8 * originalAddress
	default:
		panic("Invalid rom version")
	}
}

func (z *ZMachine) readIncPC(frame *CallStackFrame) uint8 {
	v := z.Core.ReadByte(frame.pc)
	frame.pc++
	return v
}

func (z *ZMachine) ReadHalfWordIncPC(frame *CallStackFrame) uint16 {
	v := z.Core.ReadHalfWord(frame.pc)
	frame.pc += 2
	return v
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
		return z.Core.ReadHalfWord(uint32(z.Core.GlobalVariableBase + 2*(uint16(variable)-16)))
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
		z.Core.WriteHalfWord(uint32(z.Core.GlobalVariableBase+2*(uint16(variable)-16)), value)
	}
}

func LoadRom(storyFile []uint8, inputChannel <-chan string, outputChannel chan<- any) *ZMachine {
	machine := ZMachine{
		Core:           zcore.LoadCore(storyFile),
		inputChannel:   inputChannel,
		outputChannel:  outputChannel,
		issuedWarnings: make(map[string]bool),
		streams: Streams{
			Screen:        true,
			Transcript:    false,
			Memory:        false,
			CommandScript: false,
		},
		rng: *rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Load custom alphabets on v5+
	machine.Alphabets = zstring.LoadAlphabets(&machine.Core)

	// TODO - Is the dictionary static? If not shouldn't cache like this
	machine.dictionary = dictionary.ParseDictionary(uint32(machine.Core.DictionaryBase), &machine.Core, machine.Alphabets)

	machine.Core.SetDefaultBackgroundColorNumber(2)
	machine.Core.SetDefaultForegroundColorNumber(9)
	machine.screenModel = newScreenModel(Color{0, 0, 0}, Color{255, 255, 255})

	// V6+ uses a packed address and a routine for the initial function
	if machine.Core.Version == 6 {
		packedAddress := machine.packedAddress(uint32(machine.Core.FirstInstruction), false)

		machine.callStack.push(CallStackFrame{
			pc:     packedAddress + 1,
			locals: make([]uint16, machine.Core.ReadByte(packedAddress)),
		})
	} else {
		machine.callStack.push(CallStackFrame{
			pc:     uint32(machine.Core.FirstInstruction),
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

	localVariableCount := z.Core.ReadByte(routineAddress)
	routineAddress++

	locals := make([]uint16, localVariableCount)

	for i := 0; i < int(localVariableCount); i++ {
		if i+1 < opcode.numOperands {
			// Value passed to routine, override default
			locals[i] = opcode.operands[i+1].Value(z)
		} else {
			// No value passed to routine, use default
			if z.Core.Version < 5 {
				locals[i] = z.Core.ReadHalfWord(routineAddress)
			}
		}

		if z.Core.Version < 5 {
			routineAddress += 2
		}
	}

	z.callStack.push(CallStackFrame{
		pc:              routineAddress,
		locals:          locals,
		routineStack:    make([]uint16, 0),
		routineType:     routineType, // TODO - Not really sure what this is, v3+ only
		numValuesPassed: opcode.numOperands - 1,
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
		switch offset {
		case 0:
			z.retValue(0)
		case 1:
			z.retValue(1)
		default:
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

func tokeniseSingleWord(bytes []uint8, wordStartPtr uint32, dictionary *dictionary.Dictionary, core *zcore.Core, alphabets *zstring.Alphabets) word {
	runes := []rune(string(bytes))
	zstr := zstring.Encode(runes, core, alphabets)

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
	if z.Core.Version >= 5 {
		chrCount = uint32(z.Core.ReadByte(startingLocation))
		startingLocation++
	}
	currentLocation := startingLocation

	for _, chr := range z.Core.ReadSlice(startingLocation, z.Core.MemoryLength()) {
		if (z.Core.Version < 5 && chr == 0) || (z.Core.Version >= 5 && currentLocation-(baddr1+2) >= chrCount) {
			words = append(words, tokeniseSingleWord(z.Core.ReadSlice(startingLocation, currentLocation), startingLocation, dictionary, &z.Core, z.Alphabets))
			break
		}

		if chr == ' ' { // space is always a separator
			words = append(words, tokeniseSingleWord(z.Core.ReadSlice(startingLocation, currentLocation), startingLocation, dictionary, &z.Core, z.Alphabets))
			startingLocation = currentLocation + 1
		} else {
			for _, separator := range z.dictionary.Header.InputCodes {
				if chr == separator {
					words = append(words, tokeniseSingleWord(z.Core.ReadSlice(startingLocation, currentLocation), startingLocation, dictionary, &z.Core, z.Alphabets))
					words = append(words, tokeniseSingleWord(z.Core.ReadSlice(currentLocation, currentLocation+1), startingLocation, dictionary, &z.Core, z.Alphabets))
					startingLocation = currentLocation + 1
					break
				}
			}
		}

		currentLocation += 1
		bytesRead += 1
	}

	if z.Core.ReadByte(baddr2) < uint8(len(words)) {
		panic("Error to have more words than allowed in the buffer here")
	}

	parseBufferPtr := baddr2 + 1
	z.Core.WriteByte(parseBufferPtr, uint8(len(words)))
	parseBufferPtr += 1
	for _, word := range words {
		z.Core.WriteHalfWord(parseBufferPtr, word.dictionaryAddress)
		z.Core.WriteByte(parseBufferPtr+2, uint8(len(word.bytes)))
		z.Core.WriteByte(parseBufferPtr+3, uint8(word.startingLocation-baddr1))

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
	// Undefined behaviour in spec, but Frotz just warns and does nothing
	if objId == 0 {
		z.warnOnce("remove_obj", "Warning: @remove_obj called with object 0 (PC = %x)", z.currentInstructionPC)
		return
	}

	object := zobject.GetObject(objId, &z.Core, z.Alphabets)
	if object.Parent != 0 {
		oldParent := zobject.GetObject(object.Parent, &z.Core, z.Alphabets)

		// Remove from old location in the sibling chain
		if oldParent.Child == object.Id {
			// First child case
			oldParent.SetChild(object.Sibling, &z.Core)
		} else {
			// Non-first child case - in theory can't have a sibling if no parent so no need to do this if parent == 0
			currObjId := oldParent.Child
			for {
				if currObjId == 0 {
					break
				}

				currObj := zobject.GetObject(currObjId, &z.Core, z.Alphabets)
				if currObj.Sibling == object.Id {
					currObj.SetSibling(object.Sibling, &z.Core)
					break
				} else {
					currObjId = currObj.Sibling
				}
			}
		}

		object.SetParent(0, &z.Core)
	}

	object.SetSibling(0, &z.Core)
}

func (z *ZMachine) MoveObject(objId uint16, newParent uint16) {
	// Undefined behaviour in spec, but Frotz just warns and does nothing
	if objId == 0 {
		z.warnOnce("insert_obj", "Warning: @insert_obj called with object 0 (PC = %x)", z.currentInstructionPC)
		return
	}

	// If newParent is 0, just remove the object from the tree
	if newParent == 0 {
		z.RemoveObject(objId)
		return
	}

	object := zobject.GetObject(objId, &z.Core, z.Alphabets)

	// Detach it from it's current place in the tree
	z.RemoveObject(object.Id)

	// Re-read destination's child from memory after removal, as RemoveObject may have
	// modified it (e.g., if objId was the first child of newParent)
	destChild := zobject.GetObject(newParent, &z.Core, z.Alphabets).Child

	// Set new location in the tree
	object.SetSibling(destChild, &z.Core)
	object.SetParent(newParent, &z.Core)

	// Re-read destination object to get correct base address for SetChild
	destinationObject := zobject.GetObject(newParent, &z.Core, z.Alphabets)
	destinationObject.SetChild(object.Id, &z.Core)
}

func (z *ZMachine) appendText(s string) {
	if z.streams.Memory {
		currentMemoryStream := &z.streams.MemoryStreamData[len(z.streams.MemoryStreamData)-1]
		for _, r := range s {
			z.Core.WriteByte(currentMemoryStream.ptr, uint8(r))
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
			if len(lines) > 1 {
				// Text contains newlines - Y advances by number of newlines, X resets to length of last segment
				z.screenModel.UpperWindowCursorY += len(lines) - 1
				z.screenModel.UpperWindowCursorX = len(lines[len(lines)-1])
			} else {
				// No newlines - just advance X
				z.screenModel.UpperWindowCursorX += len(s)
			}
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
	if z.Core.Version <= 3 { // TODO - Not really sure if this is true
		currentLocation := zobject.GetObject(z.readVariable(16, false), &z.Core, z.Alphabets)
		z.outputChannel <- StatusBar{
			PlaceName:   currentLocation.Name,
			Score:       int(z.readVariable(17, false)),
			Moves:       int(z.readVariable(18, false)),
			IsTimeBased: z.Core.StatusBarTimeBased,
		}
	}

	// In V5+ a custom set of terminating characters can be stored in memory
	validTerminators := []uint8{'\n'}
	if z.Core.Version >= 5 {
		if z.Core.TerminatingCharTableBase != 0 {
			//panic("TODO - Don't use this yet so panic and fix if you find a story file with this set")
			terminatingChrPtr := z.Core.TerminatingCharTableBase
			for {
				b := z.Core.ReadByte(uint32(terminatingChrPtr))
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

	bufferSize := z.Core.ReadByte(uint32(textBufferPtr))
	textBufferPtr++

	// Skip bytes already in the buffer on v5+
	if z.Core.Version >= 5 {
		existingBytes := z.Core.ReadByte(uint32(textBufferPtr))
		textBufferPtr += 1 + uint16(existingBytes)
	}

	ix := 0
	for {
		if ix > int(bufferSize) || ix >= len(rawTextBytes) { // TODO - Not 100% sure on whether this is >= or some other off by one value. Docs are unclear
			break // Too many characters provided
		}

		chr := rawTextBytes[ix]

		if (chr >= 32 && chr <= 126) || (chr >= 155 && chr <= 251) {
			z.Core.WriteByte(uint32(textBufferPtr+uint16(ix)), chr)
		} else {
			z.Core.WriteByte(uint32(textBufferPtr+uint16(ix)), 32)
		}

		ix++
	}

	// Terminate with a null byte
	z.Core.WriteByte(uint32(textBufferPtr+uint16(ix)), 0)

	// Need to store the number of bytes in total in v5+ as that's used to determine end point of the string
	if z.Core.Version >= 5 {
		z.Core.WriteByte(uint32(opcode.operands[0].Value(z)+1), uint8(ix))
	}

	// TODO - Can this ever really be zero?
	if parseBufferPtr != 0 {
		z.Tokenise(uint32(opcode.operands[0].Value(z)), uint32(parseBufferPtr), z.dictionary, false)
	}

	if z.Core.Version >= 5 {
		z.writeVariable(z.readIncPC(z.callStack.peek()), 13, false) // TODO - Should be the typed terminating char
	}
}

// reportError sends an error to the output channel and returns false to stop execution
func (z *ZMachine) reportError(format string, args ...any) bool {
	z.outputChannel <- RuntimeError(fmt.Sprintf(format, args...))
	return false
}

// warnOnce emits a warning to the output channel, but only once per warning key.
// This matches Frotz behavior: "Warning: ... (will ignore further occurrences)"
// The warningKey should be the opcode name (e.g., "test_attr")
func (z *ZMachine) warnOnce(warningKey string, format string, args ...any) {
	if z.issuedWarnings[warningKey] {
		return
	}
	z.issuedWarnings[warningKey] = true
	msg := fmt.Sprintf(format, args...)
	z.outputChannel <- Warning(msg + " (will ignore further occurrences)")
}

func (z *ZMachine) Run() {
	// Catch any remaining panics from helper functions and convert to RuntimeError
	defer func() {
		if r := recover(); r != nil {
			// Build debug context from PC history
			var debugInfo strings.Builder
			fmt.Fprintf(&debugInfo, "Internal error: %v\n", r)
			debugInfo.WriteString("Recent opcode history (most recent last):\n")
			for i := range 10 {
				idx := (pcHistoryPtr - 10 + i + 100) % 100
				op := pcHistory[idx]
				if op.pc != 0 { // Skip uninitialized entries
					fmt.Fprintf(&debugInfo, "  PC=0x%x opcode=0x%x operands=%v\n", op.pc, op.opcodeByte, op.operands)
				}
			}
			z.outputChannel <- RuntimeError(debugInfo.String())
			z.outputChannel <- Quit(true)
		}
	}()

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
	opcode := ParseOpcode(z)
	z.currentInstructionPC = opcode.pc
	frame := z.callStack.peek()

	pcHistory[pcHistoryPtr] = opcode
	pcHistoryPtr = (pcHistoryPtr + 1) % 100

	switch opcode.operandCount {
	case OP0:
		switch opcode.opcodeNumber {
		case 0: // RTRUE
			z.retValue(1)

		case 1: // RFALSE
			z.retValue(0)

		case 2: // PRINT
			text, bytesRead := zstring.Decode(frame.pc, z.Core.MemoryLength(), &z.Core, z.Alphabets, false)
			frame.pc += bytesRead
			z.appendText(text)

		case 3: // PRINT_RET
			text, bytesRead := zstring.Decode(frame.pc, z.Core.MemoryLength(), &z.Core, z.Alphabets, false)
			frame.pc += bytesRead
			z.appendText(text)
			z.appendText("\n")
			z.retValue(1)

		case 4: // NOP
			// Do nothing

		case 7: // RESTART
			z.outputChannel <- Restart(true)
			return false

		case 8: // RET_POPPED
			v := frame.pop()
			z.retValue(v)

		case 9: // POP (v1-4) / CATCH (v5+)
			if z.Core.Version <= 4 {
				frame.pop()
			} else {
				// Tag the current frame with a unique frame pointer and store it
				z.nextFramePointer++
				frame.framePointer = uint32(z.nextFramePointer)
				z.writeVariable(z.readIncPC(frame), z.nextFramePointer, false)
			}

		case 10: // QUIT
			return false

		case 11: // NEWLINE
			z.appendText("\n")

		case 13: // VERIFY
			checksum := z.Core.FileChecksum
			fileLength := z.Core.FileLength()
			actualChecksum := uint16(0)

			for ix := uint32(0x40); ix < uint32(fileLength); ix++ {
				actualChecksum += uint16(z.Core.ReadByte(ix))
			}

			z.handleBranch(frame, checksum == actualChecksum || true) // TODO - Verify doesn't really work but also not clear why we'd ever want to fail a verify test

		case 15: // PIRACY
			z.handleBranch(frame, true) // Interpreters are asked to be gullible and to unconditionally branch

		default:
			return z.reportError("OP0 opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc)
		}

	case OP1:
		switch opcode.opcodeNumber {
		case 0: // JZ
			z.handleBranch(frame, opcode.operands[0].Value(z) == 0)

		case 1: // GET_SIBLING
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("get_sibling", "Warning: @get_sibling called with object 0 (PC = %x)", opcode.pc)
			}
			sibling := zobject.GetObjectSafe(objId, &z.Core, z.Alphabets).Sibling
			z.writeVariable(z.readIncPC(frame), sibling, false)

			z.handleBranch(frame, sibling != 0)

		case 2: // GET_CHILD
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("get_child", "Warning: @get_child called with object 0 (PC = %x)", opcode.pc)
			}
			child := zobject.GetObjectSafe(objId, &z.Core, z.Alphabets).Child
			z.writeVariable(z.readIncPC(frame), child, false)

			z.handleBranch(frame, child != 0)

		case 3: // GET_PARENT
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("get_parent", "Warning: @get_parent called with object 0 (PC = %x)", opcode.pc)
			}
			z.writeVariable(z.readIncPC(frame), zobject.GetObjectSafe(objId, &z.Core, z.Alphabets).Parent, false)

		case 4: // GET_PROP_LEN
			addr := opcode.operands[0].Value(z)
			z.writeVariable(z.readIncPC(frame), zobject.GetPropertyLength(&z.Core, uint32(addr)), false)

		case 5: // INC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable, true)+1, true)

		case 6: // DEC
			variable := uint8(opcode.operands[0].Value(z))
			z.writeVariable(variable, z.readVariable(variable, true)-1, true)

		case 7: // PRINT_ADDR
			address := opcode.operands[0].Value(z)
			str, _ := zstring.Decode(uint32(address), z.Core.MemoryLength(), &z.Core, z.Alphabets, false)
			z.appendText(str)

		case 8: // CALL_1S
			z.call(&opcode, function)

		case 9: // REMOVE_OBJ
			z.RemoveObject(opcode.operands[0].Value(z))

		case 10: // PRINT_OBJ
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("print_obj", "Warning: @print_obj called with object 0 (PC = %x)", opcode.pc)
			}
			obj := zobject.GetObjectSafe(objId, &z.Core, z.Alphabets)
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
			text, _ := zstring.Decode(addr, z.Core.MemoryLength(), &z.Core, z.Alphabets, false)
			z.appendText(text)

		case 14: // LOAD
			value := opcode.operands[0].Value(z)
			z.writeVariable(z.readIncPC(frame), z.readVariable(uint8(value), true), false)

		case 15: // NOT or CALL_1n
			if z.Core.Version < 5 {
				val := opcode.operands[0].Value(z)
				z.writeVariable(z.readIncPC(frame), ^val, false)
			} else {
				z.call(&opcode, procedure)
			}
		default:
			return z.reportError("OP1 opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc)
		}

	case OP2:
		switch opcode.opcodeNumber {
		case 1: // JE
			a := opcode.operands[0].Value(z)
			branch := false
			for i := 1; i < opcode.numOperands; i++ {
				if a == opcode.operands[i].Value(z) {
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
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("jin", "Warning: @jin called with object 0 (PC = %x)", opcode.pc)
			}
			obj := zobject.GetObjectSafe(objId, &z.Core, z.Alphabets)
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
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("test_attr", "Warning: @test_attr called with object 0 (PC = %x)", opcode.pc)
			}
			obj := zobject.GetObjectSafe(objId, &z.Core, z.Alphabets)
			z.handleBranch(frame, obj.TestAttribute(opcode.operands[1].Value(z)))

		case 11: // SET_ATTR
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("set_attr", "Warning: @set_attr called with object 0 (PC = %x)", opcode.pc)
			} else {
				obj := zobject.GetObject(objId, &z.Core, z.Alphabets)
				obj.SetAttribute(opcode.operands[1].Value(z), &z.Core)
			}

		case 12: // CLEAR_ATTR
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("clear_attr", "Warning: @clear_attr called with object 0 (PC = %x)", opcode.pc)
			} else {
				obj := zobject.GetObject(objId, &z.Core, z.Alphabets)
				obj.ClearAttribute(opcode.operands[1].Value(z), &z.Core)
			}

		case 13: // STORE
			z.writeVariable(uint8(opcode.operands[0].Value(z)), opcode.operands[1].Value(z), true)

		case 14: // INSERT_OBJ
			z.MoveObject(opcode.operands[0].Value(z), opcode.operands[1].Value(z))

		case 15: // LOADW
			z.writeVariable(z.readIncPC(frame), z.Core.ReadHalfWord(uint32(opcode.operands[0].Value(z)+2*opcode.operands[1].Value(z))), false)

		case 16: // LOADB
			z.writeVariable(z.readIncPC(frame), uint16(z.Core.ReadByte(uint32(opcode.operands[0].Value(z)+opcode.operands[1].Value(z)))), false)

		case 17: // GET_PROP
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("get_prop", "Warning: @get_prop called with object 0 (PC = %x)", opcode.pc)
				z.writeVariable(z.readIncPC(frame), 0, false)
			} else {
				obj := zobject.GetObject(objId, &z.Core, z.Alphabets)
				prop := obj.GetProperty(uint8(opcode.operands[1].Value(z)), &z.Core)

				value := uint16(prop.Data[0])
				if len(prop.Data) == 2 {
					value = binary.BigEndian.Uint16(prop.Data)
				} else if len(prop.Data) > 2 {
					value = binary.BigEndian.Uint16(prop.Data[:2])
					z.warnOnce("get_prop_prop_len", "Warning: @get_prop called with object %d property %d which has length %d (PC = %x); only first two bytes returned", objId, opcode.operands[1].Value(z), len(prop.Data), opcode.pc)
				}

				z.writeVariable(z.readIncPC(frame), value, false)
			}

		case 18: // GET_PROP_ADDR
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("get_prop_addr", "Warning: @get_prop_addr called with object 0 (PC = %x)", opcode.pc)
				z.writeVariable(z.readIncPC(frame), 0, false)
			} else {
				obj := zobject.GetObject(objId, &z.Core, z.Alphabets)
				prop := obj.GetProperty(uint8(opcode.operands[1].Value(z)), &z.Core)
				z.writeVariable(z.readIncPC(frame), uint16(prop.DataAddress), false)
			}

		case 19: // GET_NEXT_PROP
			objId := opcode.operands[0].Value(z)
			if objId == 0 {
				z.warnOnce("get_next_prop", "Warning: @get_next_prop called with object 0 (PC = %x)", opcode.pc)
				z.writeVariable(z.readIncPC(frame), 0, false)
			} else {
				obj := zobject.GetObject(objId, &z.Core, z.Alphabets)
				nextProp := obj.GetNextProperty(uint8(opcode.operands[1].Value(z)), &z.Core)
				z.writeVariable(z.readIncPC(frame), uint16(nextProp), false)
			}

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
				return z.reportError("Division by zero")
			}
			z.writeVariable(z.readIncPC(frame), uint16(numerator/denominator), false)

		case 24: // MOD
			numerator := int16(opcode.operands[0].Value(z))
			denominator := int16(opcode.operands[1].Value(z))
			if denominator == 0 {
				return z.reportError("Modulo by zero")
			}
			z.writeVariable(z.readIncPC(frame), uint16(numerator%denominator), false)

		case 25: // call_2s
			if z.Core.Version < 4 {
				return z.reportError("call_2s not available on v1-3")
			}

			z.call(&opcode, function)

		case 26: // CALL_2n
			if z.Core.Version < 5 {
				return z.reportError("call_2n not available on v1-4")
			}

			z.call(&opcode, procedure)

		case 27: // set_colour
			if z.Core.Version < 5 {
				return z.reportError("set_colour not available on v1-4")
			}

			foreground := z.screenModel.NewZMachineColor(opcode.operands[0].Value(z), true)
			background := z.screenModel.NewZMachineColor(opcode.operands[1].Value(z), false)
			if z.screenModel.LowerWindowActive {
				z.screenModel.LowerWindowForeground = foreground
				z.screenModel.LowerWindowBackground = background
			} else {
				z.screenModel.UpperWindowForeground = foreground
				z.screenModel.UpperWindowBackground = background
			}
			z.outputChannel <- z.screenModel

		case 28: // throw
			if z.Core.Version < 5 {
				return z.reportError("throw not available on v1-4")
			}
			returnValue := opcode.operands[0].Value(z)
			fp := uint32(opcode.operands[1].Value(z))

			// Pop frames until we find the one with the matching frame pointer
			for z.callStack.peek().framePointer != fp {
				z.callStack.pop()
			}

			// Return with the given value from the found frame
			z.retValue(returnValue)

		case 0, 29, 30, 31: // blank
			return z.reportError("Unused 2OP opcode number: 0x%x", opcode.opcodeNumber)

		default:
			return z.reportError("Invalid 2OP state, interpreter bug")
		}

	case VAR:
		if opcode.opcodeForm == extForm {
			switch opcode.opcodeByte {
			case 0x00:
				return z.reportError("EXT save not implemented")
			case 0x01:
				return z.reportError("EXT restore not implemented")
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

			case 0x04: // SET_FONT
				requestFont := Font(opcode.operands[0].Value(z))

				// V6 has optional window parameter - we don't support multiple windows
				if z.Core.Version == 6 && opcode.numOperands > 1 {
					window := int16(opcode.operands[1].Value(z))
					if window != -3 && window != 0 {
						z.warnOnce("set_font_v6_window", "Warning: SET_FONT with window %d not supported (only -3 and 0)", window)
					}
				}

				previousFont := z.screenModel.CurrentFont
				var result uint16

				switch requestFont {
				case 0:
					// Font 0: return current font, don't change
					result = uint16(previousFont)
				case FontNormal, FontFixedPitch:
					// Available fonts
					z.screenModel.CurrentFont = requestFont
					result = uint16(previousFont)
				default:
					// FontPicture, FontCharGraphs, and others: unavailable
					result = 0
				}

				z.writeVariable(z.readIncPC(frame), result, false)
				z.outputChannel <- z.screenModel

			case 0x09: // SAVE_UNDO
				z.saveUndo()
				z.writeVariable(z.readIncPC(frame), uint16(1), false) // Save always succeeds in this environment

			case 0x0a: // RESTORE_UNDO
				response := z.restoreUndo()
				frame = z.callStack.peek()
				z.writeVariable(z.readIncPC(frame), response, false) // Restore always says that it's done and continues from previous save

			case 0x0b: // PRINT_UNICODE
				chr := opcode.operands[0].Value(z)
				z.appendText(string(rune(chr)))

			case 0x0c: // CHECK_UNICODE
				chr := opcode.operands[0].Value(z)
				// What unicode characters _can_ i write? TODO
				if chr != 0 {
					z.writeVariable(z.readIncPC(frame), 0b11, false)
				}

			case 0x0d: // SET_TRUE_COLOUR
				foreground := opcode.operands[0].Value(z)
				background := opcode.operands[1].Value(z)
				var fgColor, bgColor Color

				if int16(foreground) == -1 {
					if z.screenModel.LowerWindowActive {
						fgColor = z.screenModel.DefaultLowerWindowForeground
					} else {
						fgColor = z.screenModel.DefaultUpperWindowForeground
					}
				} else if int16(foreground) == -2 {
					if z.screenModel.LowerWindowActive {
						fgColor = z.screenModel.LowerWindowForeground
					} else {
						fgColor = z.screenModel.UpperWindowForeground
					}
				} else {
					fgColor = Color{int(foreground&0b11111) * 32, int((foreground>>5)&0b11111) * 32, int((foreground>>10)&0b11111) * 32}
				}

				if int16(background) == -1 {
					if z.screenModel.LowerWindowActive {
						bgColor = z.screenModel.DefaultLowerWindowBackground
					} else {
						bgColor = z.screenModel.DefaultUpperWindowBackground
					}
				} else if int16(foreground) == -2 {
					if z.screenModel.LowerWindowActive {
						bgColor = z.screenModel.LowerWindowBackground
					} else {
						bgColor = z.screenModel.UpperWindowBackground
					}
				} else {
					bgColor = Color{int(background&0b11111) * 32, int((background>>5)&0b11111) * 32, int((background>>10)&0b11111) * 32}
				}

				if z.screenModel.LowerWindowActive {
					z.screenModel.LowerWindowForeground = fgColor
					z.screenModel.LowerWindowBackground = bgColor
				} else {
					z.screenModel.UpperWindowForeground = fgColor
					z.screenModel.UpperWindowBackground = bgColor
				}

				z.outputChannel <- z.screenModel

			default:
				return z.reportError("EXT opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc)
			}
		} else {
			switch opcode.opcodeNumber {
			case 0: // CALL
				z.call(&opcode, function)

			case 1: // STOREW
				address := opcode.operands[0].Value(z) + 2*opcode.operands[1].Value(z)
				value := opcode.operands[2].Value(z)
				z.Core.WriteHalfWord(uint32(address), value)

			case 2: // STOREB
				address := opcode.operands[0].Value(z) + opcode.operands[1].Value(z)
				z.Core.WriteByte(uint32(address), uint8(opcode.operands[2].Value(z)))

			case 3: // PUT_PROP
				objId := opcode.operands[0].Value(z)
				if objId == 0 {
					z.warnOnce("put_prop", "Warning: @put_prop called with object 0 (PC = %x)", opcode.pc)
				} else {
					obj := zobject.GetObject(objId, &z.Core, z.Alphabets)
					obj.SetProperty(uint8(opcode.operands[1].Value(z)), opcode.operands[2].Value(z), &z.Core)
				}

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
					result = uint16(z.rng.Int31n(int32(n))) + 1
				}

				z.writeVariable(z.readIncPC(frame), result, false)
			case 8: // PUSH
				frame.push(opcode.operands[0].Value(z))

			case 9: // PULL
				if z.Core.Version == 6 {
					if opcode.numOperands > 0 {
						return z.reportError("V6 PULL with user stack not implemented")
					}
					value := frame.pop()
					z.writeVariable(z.readIncPC(frame), value, false)
				} else {
					z.writeVariable(uint8(opcode.operands[0].Value(z)), frame.pop(), true)
				}

			case 10: // SPLIT_WINDOW
				if z.Core.Version < 3 {
					return z.reportError("SPLIT_WINDOW not available on v1-2")
				}

				lines := opcode.operands[0].Value(z)
				z.screenModel.UpperWindowHeight = int(lines)

				z.outputChannel <- z.screenModel

			case 11: // SET_WINDOW
				if z.Core.Version < 3 {
					return z.reportError("SET_WINDOW not available on v1-2")
				}
				window := opcode.operands[0].Value(z)
				z.screenModel.LowerWindowActive = window == 0
				// 8.7.2: Whenever the upper window is selected, its cursor position is reset to the top left
				if window == 1 {
					z.screenModel.UpperWindowCursorX = 0
					z.screenModel.UpperWindowCursorY = 0
				}
				z.outputChannel <- z.screenModel

			case 12: // CALL_VS2
				z.call(&opcode, function)

			case 13: // ERASE_WINDOW
				window := int16(opcode.operands[0].Value(z))

				switch window {
				case -1:
					// 8.7.3.3: Clear whole screen, collapse upper window to height 0, select lower window
					z.screenModel.UpperWindowHeight = 0
					z.screenModel.LowerWindowActive = true
					// Move cursor to top left (V5+) or bottom left (V4)
					// For now assuming V5+ behavior
					z.screenModel.UpperWindowCursorX = 0
					z.screenModel.UpperWindowCursorY = 0
				case -2:
					// Keep split but clear both windows
					// 8.7.3.2.1: Cursor moves to top left (V5+)
					z.screenModel.UpperWindowCursorX = 0
					z.screenModel.UpperWindowCursorY = 0
				case 0:
					// Clear lower window - cursor handled by UI
				case 1:
					// Clear upper window
					// 8.7.3.2.1: Cursor moves to top left
					z.screenModel.UpperWindowCursorX = 0
					z.screenModel.UpperWindowCursorY = 0
				}

				z.outputChannel <- z.screenModel
				z.outputChannel <- EraseWindowRequest(window)

			case 14: // ERASE_LINE
				if z.Core.Version < 4 {
					return z.reportError("ERASE_LINE not available on v1-3")
				}

				value := int16(opcode.operands[0].Value(z))
				switch value {
				case 1:
					// Erase from cursor to end of line
					z.outputChannel <- EraseLineRequest(1)
				default:
					// "If the value is anything other than 1, do nothing." - Spec
				}

			case 15: // SET_CURSOR
				line := opcode.operands[0].Value(z)
				col := opcode.operands[1].Value(z)

				if z.Core.Version == 6 {
					return z.reportError("V6 cursor operations not implemented")
				}

				// TODO - Pretty sure you can't set the cursor on lower window v<=5
				// Z-machine uses 1-based coordinates, convert to 0-based
				if !z.screenModel.LowerWindowActive {
					z.screenModel.UpperWindowCursorX = int(col) - 1
					z.screenModel.UpperWindowCursorY = int(line) - 1
					z.outputChannel <- z.screenModel
				}

			case 16: // GET_CURSOR
				if z.Core.Version == 6 {
					return z.reportError("V6 cursor operations not implemented")
				}

				array := uint32(opcode.operands[0].Value(z))
				// Store cursor position as 1-based coordinates (Z-machine convention)
				// Word 0 = row, Word 1 = column
				z.Core.WriteHalfWord(array, uint16(z.screenModel.UpperWindowCursorY+1))
				z.Core.WriteHalfWord(array+2, uint16(z.screenModel.UpperWindowCursorX+1))

			case 17: // SET_TEXT_STYLE
				if z.Core.Version >= 4 {
					mask := uint8(opcode.operands[0].Value(z))

					if z.screenModel.LowerWindowActive {
						z.screenModel.LowerWindowTextStyle = TextStyle(mask)
					} else {
						z.screenModel.UpperWindowTextStyle = TextStyle(mask)
					}

					z.outputChannel <- z.screenModel
				} else {
					return z.reportError("SET_TEXT_STYLE not available on v1-3")
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
						z.Core.WriteHalfWord(sizeWordAddress, uint16(currentActiveStream.ptr-currentActiveStream.baseAddress-2))

						// Note that there might be historical streams still active, these act as a stack
						z.streams.MemoryStreamData = z.streams.MemoryStreamData[:len(z.streams.MemoryStreamData)-1]
						if len(z.streams.MemoryStreamData) == 0 {
							z.streams.Memory = false
						}
					}
				case 4, -4:
					z.streams.CommandScript = stream > 0
				}

			case 22: // READ_CHAR
				z.outputChannel <- WaitForCharacter
				rawText := <-z.inputChannel

				// Handle empty input (treat as newline)
				charCode := uint16(13) // Default to carriage return
				if len(rawText) > 0 {
					charCode = uint16(rawText[0])
				}
				z.writeVariable(z.readIncPC(frame), charCode, false)

			case 23: // SCAN_TABLE
				test := opcode.operands[0].Value(z)
				tableAddress := opcode.operands[1].Value(z)
				length := opcode.operands[2].Value(z)
				form := uint16(0x82)

				if opcode.numOperands == 4 {
					form = opcode.operands[3].Value(z)
				}

				result := ztable.ScanTable(&z.Core, test, uint32(tableAddress), length, form)

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

				if opcode.numOperands > 2 {
					dictionaryAddress := opcode.operands[2].Value(z)

					// TODO - Handle special case custom dictionaries with negative number of entries (unsorted)
					dictionaryToUse = dictionary.ParseDictionary(uint32(dictionaryAddress), &z.Core, z.Alphabets)

					if opcode.numOperands == 4 {
						flag = opcode.operands[3].Value(z) != 0

						return z.reportError("TOKENISE with 4th operand not implemented")
					}
				}

				z.Tokenise(uint32(text), uint32(parseBuffer), dictionaryToUse, flag)

			case 29: // COPY_TABLE
				ztable.CopyTable(&z.Core, opcode.operands[0].Value(z), opcode.operands[1].Value(z), int16(opcode.operands[2].Value(z)))

			case 30: // PRINT_TABLE
				addr := opcode.operands[0].Value(z)
				width := opcode.operands[1].Value(z)
				height := uint16(1)
				skip := uint16(0)

				if opcode.numOperands > 2 {
					height = opcode.operands[2].Value(z)

					if opcode.numOperands > 3 {
						skip = opcode.operands[3].Value(z)
					}
				}
				z.appendText(ztable.PrintTable(&z.Core, uint32(addr), width, height, skip))

			case 31: // CHECK_ARG_COUNT
				arg := opcode.operands[0].Value(z)
				branch := arg <= uint16(frame.numValuesPassed)

				z.handleBranch(frame, branch)

			default:
				return z.reportError("VAR opcode not implemented 0x%x at 0x%x", opcode.opcodeByte, z.callStack.peek().pc)
			}
		}
	}

	return true
}
