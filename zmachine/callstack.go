package zmachine

import "fmt"

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

func (f *CallStackFrame) pop(z *ZMachine) uint16 {
	if len(f.routineStack) == 0 {
		z.warnOnce("stack_underflow_pop", "Warning: Attempt to pop from empty routine stack (PC = %x)", z.currentInstructionPC)
		return 0
	}
	i := f.routineStack[len(f.routineStack)-1]
	f.routineStack = f.routineStack[:len(f.routineStack)-1]
	return i
}

func (f *CallStackFrame) peek(z *ZMachine) uint16 {
	if len(f.routineStack) == 0 {
		z.warnOnce("stack_underflow_peek", "Warning: Attempt to peek from empty routine stack (PC = %x)", z.currentInstructionPC)
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

func (s *CallStack) pop() (CallStackFrame, error) {
	if len(s.frames) == 0 {
		return CallStackFrame{}, fmt.Errorf("attempt to pop from empty call stack")
	}
	stackSize := len(s.frames)
	frame := s.frames[stackSize-1]
	s.frames = s.frames[:stackSize-1]

	return frame, nil
}

func (s *CallStack) peek() (*CallStackFrame, error) {
	if len(s.frames) == 0 {
		return nil, fmt.Errorf("attempt to peek empty call stack")
	}
	return &s.frames[len(s.frames)-1], nil
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
