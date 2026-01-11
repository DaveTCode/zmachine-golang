package zmachine

type Save struct {
	Prompt   bool
	Filename string
	Address  uint32 // 0 means full save
	NumBytes uint32 // 0 means full save
}

type Restore struct {
	Prompt   bool
	Filename string
	Address  uint32 // 0 means full restore
	NumBytes uint32 // 0 means full restore
}

type SaveRestoreResponse interface {
	isSaveRestoreResponse()
}

type SaveResponse struct {
	Success bool
	Result  uint16 // 0 = failure, 1 = success
}

func (SaveResponse) isSaveRestoreResponse() {}

type RestoreResponse struct {
	Success bool
	Result  uint16 // 0 = failure, 2 = success; for auxiliary: bytes loaded
	Data    []byte // Save file bytes for full restore
}

func (RestoreResponse) isSaveRestoreResponse() {}

type SaveState struct {
	staticMemoryBase uint16
	dynamicMemory    []uint8
	callStack        CallStack
}

type InMemorySaveStateCache struct {
	saveStates []SaveState
}

func (z *ZMachine) captureState() SaveState {
	dynamicMemory := make([]uint8, z.Core.StaticMemoryBase)
	copy(dynamicMemory, z.Core.ReadSlice(0, uint32(z.Core.StaticMemoryBase)))

	return SaveState{
		staticMemoryBase: z.Core.StaticMemoryBase,
		dynamicMemory:    dynamicMemory,
		callStack:        z.callStack.copy(),
	}
}

func (z *ZMachine) applyState(state SaveState) bool {
	if state.staticMemoryBase != z.Core.StaticMemoryBase {
		return false
	}

	// TODO: retain transcription and fixed font bits per spec
	copy(z.Core.ReadSlice(0, uint32(z.Core.StaticMemoryBase)), state.dynamicMemory)
	z.callStack = state.callStack.copy()
	return true
}

func (z *ZMachine) saveUndo() {
	z.UndoStates.saveStates = append(z.UndoStates.saveStates, z.captureState())
}

func (z *ZMachine) restoreUndo() uint16 {
	if len(z.UndoStates.saveStates) == 0 {
		return 0
	}

	state := z.UndoStates.saveStates[len(z.UndoStates.saveStates)-1]
	z.UndoStates.saveStates = z.UndoStates.saveStates[:len(z.UndoStates.saveStates)-1]

	if !z.applyState(state) {
		return 0
	}
	return 2
}

// readSaveFilename reads a length-prefixed ASCII string (not a Z-string) per spec S7.6.
func (z *ZMachine) readSaveFilename(address uint32) string {
	if address == 0 {
		return ""
	}

	length := z.Core.ReadZByte(address)
	if length == 0 {
		return ""
	}

	bytes := make([]byte, length)
	for i := range length {
		bytes[i] = z.Core.ReadZByte(address + 1 + uint32(i))
	}
	return string(bytes)
}

func (z *ZMachine) ExportSaveState() []byte {
	return z.captureState().serialize()
}

func (z *ZMachine) ImportSaveState(data []byte) bool {
	state, ok := deserializeSaveState(data)
	if !ok {
		return false
	}
	return z.applyState(state)
}

// Save format "GOZM": magic(4) + staticBase(2) + dynamicMem + frameCount(2) + frames
func (s SaveState) serialize() []byte {
	frameData := s.callStack.serialize()
	data := make([]byte, 4+2+len(s.dynamicMemory)+2+len(frameData))
	offset := 0

	copy(data[offset:], []byte("GOZM"))
	offset += 4

	data[offset] = byte(s.staticMemoryBase >> 8)
	data[offset+1] = byte(s.staticMemoryBase & 0xFF)
	offset += 2

	copy(data[offset:], s.dynamicMemory)
	offset += len(s.dynamicMemory)

	frameCount := len(s.callStack.frames)
	data[offset] = byte(frameCount >> 8)
	data[offset+1] = byte(frameCount & 0xFF)
	offset += 2

	copy(data[offset:], frameData)
	return data
}

func deserializeSaveState(data []byte) (SaveState, bool) {
	if len(data) < 8 || string(data[0:4]) != "GOZM" {
		return SaveState{}, false
	}

	offset := 4
	staticBase := uint16(data[offset])<<8 | uint16(data[offset+1])
	offset += 2

	if len(data) < offset+int(staticBase)+2 {
		return SaveState{}, false
	}

	dynamicMem := make([]uint8, staticBase)
	copy(dynamicMem, data[offset:offset+int(staticBase)])
	offset += int(staticBase)

	frameCount := int(data[offset])<<8 | int(data[offset+1])
	offset += 2

	frames, _ := deserializeCallStack(data[offset:], frameCount)
	if frames == nil {
		return SaveState{}, false
	}

	return SaveState{
		staticMemoryBase: staticBase,
		dynamicMemory:    dynamicMem,
		callStack:        CallStack{frames: frames},
	}, true
}

func (cs *CallStack) serialize() []byte {
	var result []byte
	for _, frame := range cs.frames {
		result = append(result, frame.serialize()...)
	}
	return result
}

// Frame format: pc(4) + framePointer(4) + routineType(1) + numValuesPassed(2) +
// localsCount(2) + locals + stackSize(2) + stack
func (f *CallStackFrame) serialize() []byte {
	size := 4 + 4 + 1 + 2 + 2 + len(f.locals)*2 + 2 + len(f.routineStack)*2
	data := make([]byte, size)
	offset := 0

	data[offset] = byte(f.pc >> 24)
	data[offset+1] = byte(f.pc >> 16)
	data[offset+2] = byte(f.pc >> 8)
	data[offset+3] = byte(f.pc)
	offset += 4

	data[offset] = byte(f.framePointer >> 24)
	data[offset+1] = byte(f.framePointer >> 16)
	data[offset+2] = byte(f.framePointer >> 8)
	data[offset+3] = byte(f.framePointer)
	offset += 4

	data[offset] = byte(f.routineType)
	offset++

	data[offset] = byte(f.numValuesPassed >> 8)
	data[offset+1] = byte(f.numValuesPassed)
	offset += 2

	data[offset] = byte(len(f.locals) >> 8)
	data[offset+1] = byte(len(f.locals))
	offset += 2
	for _, local := range f.locals {
		data[offset] = byte(local >> 8)
		data[offset+1] = byte(local)
		offset += 2
	}

	data[offset] = byte(len(f.routineStack) >> 8)
	data[offset+1] = byte(len(f.routineStack))
	offset += 2
	for _, val := range f.routineStack {
		data[offset] = byte(val >> 8)
		data[offset+1] = byte(val)
		offset += 2
	}

	return data
}

func deserializeCallStack(data []byte, frameCount int) ([]CallStackFrame, int) {
	frames := make([]CallStackFrame, 0, frameCount)
	offset := 0

	for range frameCount {
		if offset+13 > len(data) {
			return nil, 0
		}

		frame := CallStackFrame{}

		frame.pc = uint32(data[offset])<<24 | uint32(data[offset+1])<<16 |
			uint32(data[offset+2])<<8 | uint32(data[offset+3])
		offset += 4

		frame.framePointer = uint32(data[offset])<<24 | uint32(data[offset+1])<<16 |
			uint32(data[offset+2])<<8 | uint32(data[offset+3])
		offset += 4

		frame.routineType = RoutineType(data[offset])
		offset++

		frame.numValuesPassed = int(data[offset])<<8 | int(data[offset+1])
		offset += 2

		localCount := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		if offset+localCount*2 > len(data) {
			return nil, 0
		}
		frame.locals = make([]uint16, localCount)
		for j := range localCount {
			frame.locals[j] = uint16(data[offset])<<8 | uint16(data[offset+1])
			offset += 2
		}

		if offset+2 > len(data) {
			return nil, 0
		}
		stackSize := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		if offset+stackSize*2 > len(data) {
			return nil, 0
		}
		frame.routineStack = make([]uint16, stackSize)
		for j := range stackSize {
			frame.routineStack[j] = uint16(data[offset])<<8 | uint16(data[offset+1])
			offset += 2
		}

		frames = append(frames, frame)
	}

	return frames, offset
}
