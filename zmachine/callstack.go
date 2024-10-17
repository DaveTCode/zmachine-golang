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

func (f *CallStackFrame) pop() uint16 {
	i := f.routineStack[len(f.routineStack)-1]
	f.routineStack = f.routineStack[:len(f.routineStack)-1]
	return i
}

func (f *CallStackFrame) peek() uint16 {
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

		for rx, r := range frame.routineStack {
			copiedFrame.routineStack[rx] = r
		}

		for lx, l := range frame.locals {
			copiedFrame.locals[lx] = l
		}

		callStack.frames[fx] = copiedFrame
	}

	return callStack
}
