package zmachine

type SaveState struct {
	dynamicMemory []uint8
	callStack     CallStack
}

type InMemorySaveStateCache struct {
	saveStates []SaveState
}

func (z *ZMachine) saveUndo() {
	// Take copy of dynamic memory
	dynamicMemory := make([]uint8, z.Core.StaticMemoryBase)
	copy(dynamicMemory, z.Core.ReadSlice(0, uint32(z.Core.StaticMemoryBase)))

	z.UndoStates.saveStates = append(z.UndoStates.saveStates, SaveState{
		dynamicMemory: dynamicMemory,
		callStack:     z.callStack.copy(),
	})
}

func (z *ZMachine) restoreUndo() uint16 {
	if len(z.UndoStates.saveStates) == 0 {
		return 0
	}

	state := z.UndoStates.saveStates[len(z.UndoStates.saveStates)-1]
	z.UndoStates.saveStates = z.UndoStates.saveStates[:len(z.UndoStates.saveStates)-1]

	// Copy the old dynamic memory back in
	// TODO - in theory need to retain bits about transcription and fixed font
	copy(z.Core.ReadSlice(0, uint32(z.Core.StaticMemoryBase)), state.dynamicMemory)

	z.callStack = state.callStack.copy()

	return 2
}
