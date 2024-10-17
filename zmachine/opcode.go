package zmachine

type OperandType int
type OpcodeForm int
type OperandCount int

const (
	largeConstant OperandType = 0b00
	smallConstant OperandType = 0b01
	variable      OperandType = 0b10
	omitted       OperandType = 0b11
)

const (
	longForm  OpcodeForm = 0b00
	extForm   OpcodeForm = 0b01
	shortForm OpcodeForm = 0b10
	varForm   OpcodeForm = 0b11
)

const (
	OP0 OperandCount = iota
	OP1 OperandCount = iota
	OP2 OperandCount = iota
	VAR OperandCount = iota
	EXT OperandCount = iota
)

type Operand struct {
	operandType OperandType
	value       uint16 // Can be byte, half word or reference to variable based on operandType
}

func (operand *Operand) Value(z *ZMachine) uint16 {
	switch operand.operandType {
	case largeConstant, smallConstant:
		return operand.value
	case variable:
		return z.readVariable(uint8(operand.value), false)
	default:
		return 0
	}
}

type Opcode struct {
	opcodeByte   uint8
	operandCount OperandCount
	opcodeForm   OpcodeForm
	opcodeNumber uint8
	operands     []Operand
}

func parseVariableOperands(z *ZMachine, frame *CallStackFrame, opcode *Opcode) {
	operandTypeByte := z.readIncPC(frame)
	operandTypeByteExtendedCall := uint8(0)
	maxVariables := 4

	if (opcode.opcodeNumber == 12 || opcode.opcodeNumber == 26) && opcode.operandCount == VAR {
		operandTypeByteExtendedCall = z.readIncPC(frame)
		maxVariables = 8
	}

	for varIx := 0; varIx < maxVariables; varIx++ {
		var operandType OperandType
		if varIx < 4 {
			operandType = OperandType((operandTypeByte >> (2 * (3 - varIx))) & 0b11)
		} else {
			operandType = OperandType((operandTypeByteExtendedCall >> (2 * (7 - varIx))) & 0b11)
		}

		if operandType == omitted { // No more variables
			break
		}

		switch operandType {
		case smallConstant, variable:
			opcode.operands = append(opcode.operands, Operand{operandType: operandType, value: uint16(z.readIncPC(frame))})
		case largeConstant:
			opcode.operands = append(opcode.operands, Operand{operandType: operandType, value: z.readHalfWordIncPC(frame)})
		}
	}
}

func ParseOpcode(z *ZMachine) Opcode {
	frame := z.callStack.peek()
	opcodeByte := z.readIncPC(frame)
	opcode := Opcode{
		opcodeForm: OpcodeForm(opcodeByte >> 6),
		opcodeByte: opcodeByte,
	}

	// First decode the opcode type (Short, Long, Variable, Extended (v5+))
	if opcodeByte == 0xbe && z.Version() >= 5 {
		opcode.opcodeByte = z.readIncPC(frame)
		opcode.opcodeNumber = opcode.opcodeByte
		opcode.opcodeForm = extForm
		opcode.operandCount = VAR

		parseVariableOperands(z, frame, &opcode)
	} else if opcode.opcodeForm == varForm {
		opcode.opcodeNumber = opcodeByte & 0b1_1111 // 5 bits
		opcode.operandCount = VAR
		if ((opcodeByte >> 5) & 1) == 0 {
			opcode.operandCount = OP2
		}

		parseVariableOperands(z, frame, &opcode)
	} else if opcode.opcodeForm == shortForm {
		opcode.opcodeNumber = opcodeByte & 0b1111 // 4 bits
		operandType := (opcodeByte >> 4) & 0b11

		switch operandType {
		case 0b00: // Large Constant (2 bytes)
			opcode.operands = append(opcode.operands, Operand{operandType: OperandType(operandType), value: z.readHalfWordIncPC(frame)})
			opcode.operandCount = OP1
		case 0b01, 0b10: // Small constant or variable
			opcode.operands = append(opcode.operands, Operand{operandType: OperandType(operandType), value: uint16(z.readIncPC(frame))})
			opcode.operandCount = OP1
		case 0b11: // Omitted
			opcode.operandCount = OP0
		}
	} else { // LONG
		opcode.opcodeNumber = opcodeByte & 0b1_1111 // 5 bits
		opcode.opcodeForm = longForm
		opcode.operandCount = OP2

		operand1Type := smallConstant
		operand2Type := smallConstant
		if (opcodeByte>>6)&0b1 == 0b1 {
			operand1Type = variable
		}
		if (opcodeByte>>5)&0b1 == 0b1 {
			operand2Type = variable
		}

		for _, operandType := range []OperandType{operand1Type, operand2Type} {
			opcode.operands = append(opcode.operands, Operand{operandType: operandType, value: uint16(z.readIncPC(frame))})
		}
	}

	return opcode
}
