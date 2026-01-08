package zmachine

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

// popWithWarning pops a value from the routine stack with bounds checking.
// If the stack is empty, warns and returns 0.
func (f *CallStackFrame) popWithWarning(z *ZMachine) uint16 {
	if len(f.routineStack) == 0 {
		z.warnOnce("stack_underflow_pop", "Warning: Attempt to pop from empty routine stack (PC = %x)", z.currentInstructionPC)
		return 0
	}
	i := f.routineStack[len(f.routineStack)-1]
	f.routineStack = f.routineStack[:len(f.routineStack)-1]
	return i
}

// pop pops a value from the routine stack with silent bounds checking.
// Returns 0 if stack is empty without warning.
// Use popWithWarning instead for opcode implementations.
// This method exists for backward compatibility with readVariable which has its own warning.
func (f *CallStackFrame) pop() uint16 {
	if len(f.routineStack) == 0 {
		return 0
	}
	i := f.routineStack[len(f.routineStack)-1]
	f.routineStack = f.routineStack[:len(f.routineStack)-1]
	return i
}

// peekWithWarning peeks at the top value of the routine stack with bounds checking.
// If the stack is empty, warns and returns 0.
func (f *CallStackFrame) peekWithWarning(z *ZMachine) uint16 {
	if len(f.routineStack) == 0 {
		z.warnOnce("stack_underflow_peek", "Warning: Attempt to peek empty routine stack (PC = %x)", z.currentInstructionPC)
		return 0
	}
	return f.routineStack[len(f.routineStack)-1]
}

// peek peeks at the top value of the routine stack with silent bounds checking.
// Returns 0 if stack is empty without warning.
// Use peekWithWarning instead for opcode implementations.
// This method exists for backward compatibility with readVariable which has its own warning.
func (f *CallStackFrame) peek() uint16 {
	if len(f.routineStack) == 0 {
		return 0
	}
	return f.routineStack[len(f.routineStack)-1]
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

// CallStack - Deep copy of a call stack and all frames
func (s *CallStack) copy() CallStack {
	callStack := CallStack{
		frames: make([]CallStackFrame, len(s.frames)),
	}

	for fx, frame := range s.frames {
		copiedFrame := CallStackFrame{
			pc:              frame.pc,
			routineType:     frame.routineType,
			numValuesPassed: frame.numValuesPassed,
			framePointer:    frame.framePointer,
			routineStack:    make([]uint16, len(frame.routineStack)),
			locals:          make([]uint16, len(frame.locals)),
		}

		copy(copiedFrame.routineStack, frame.routineStack)
		copy(copiedFrame.locals, frame.locals)

		callStack.frames[fx] = copiedFrame
	}

	return callStack
}
