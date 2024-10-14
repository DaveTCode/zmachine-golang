package zobject

import (
	"encoding/binary"
	"fmt"
)

type Property struct {
	Id                   uint8
	Length               uint8
	Data                 []uint8
	PropertyHeaderLength uint8
	Address              uint32
	DataAddress          uint32
}

// Property length is requested by the address of the first byte of the data
// This function therefore works back from that to find the property length
// based on the flags set on the property size byte(s)
func GetPropertyLength(memory []uint8, addr uint32, version uint8) uint16 {
	if addr == 0 {
		return 0 // Special case required by some story files
	}

	prevByte := memory[addr-1]
	if version <= 3 {
		return uint16(prevByte>>5) + 1
	} else if memory[addr-2]&0b1000_0000 != 0 {
		return uint16(prevByte & 0b11_1111)
	} else {
		if memory[addr-2]&0b100_0000 == 0 {
			return 1
		} else {
			return 2
		}
	}
}

func (o *Object) SetProperty(propertyId uint8, value uint16, memory []uint8, version uint8, objectTableBase uint16) {
	objectNameLength := memory[uint32(o.PropertyPointer)]
	currentPtr := uint32(o.PropertyPointer + 1 + uint16(objectNameLength)*2)

	for {
		if memory[currentPtr] == 0 {
			break
		}

		property := o.GetPropertyByAddress(currentPtr, memory, version)

		if property.Id == propertyId {
			switch property.Length {
			case 1:
				memory[currentPtr+1] = uint8(value)
			case 2:
				binary.BigEndian.PutUint16(memory[currentPtr+1:currentPtr+3], value)
			default:
				panic(fmt.Sprintf("Invalid property length %d, can't set value", propertyId))
			}

			return
		}

		currentPtr += uint32(property.Length) + uint32(property.PropertyHeaderLength)
	}

	// Property not found on object, returning global default for that property
	panic(fmt.Sprintf("Invalid property (%d) requested for object (%d)", propertyId, o.Id))
}

func (o *Object) GetProperty(propertyId uint8, memory []uint8, version uint8, objectTableBase uint16) Property {
	objectNameLength := memory[uint32(o.PropertyPointer)]
	currentPtr := uint32(o.PropertyPointer + 1 + uint16(objectNameLength)*2)

	for {
		// Property table ends with null terminator
		if memory[currentPtr] == 0 {
			break
		}

		property := o.GetPropertyByAddress(currentPtr, memory, version)

		if property.Id == propertyId {
			return property
		}

		currentPtr += uint32(property.Length) + uint32(property.PropertyHeaderLength)
	}

	// Property not found on object, returning global default for that property
	propertyAddress := objectTableBase + 2*uint16(propertyId-1)
	return Property{
		Id:   propertyId,
		Data: memory[propertyAddress : propertyAddress+2],
	}
}

func (o *Object) GetPropertyByAddress(propertyAddr uint32, memory []uint8, version uint8) Property {
	propertySizeByte := memory[propertyAddr]
	length := (propertySizeByte >> 5) + 1
	id := propertySizeByte & 0b1_1111
	propertyHeaderLength := uint8(1)

	if version >= 4 {
		if propertySizeByte>>7 == 1 {
			length = (memory[propertyAddr+1] & 0b11_1111) + 1
			id = propertySizeByte & 0b11_1111
			propertyHeaderLength = 2
		} else {
			length = ((propertySizeByte >> 6) & 1) + 1
			id = propertySizeByte & 0b11_1111
		}
	}

	dataAddress := propertyAddr + uint32(propertyHeaderLength)

	return Property{
		Id:                   id,
		Length:               length,
		Data:                 memory[dataAddress : dataAddress+uint32(length)],
		PropertyHeaderLength: propertyHeaderLength,
		Address:              propertyAddr,
		DataAddress:          dataAddress,
	}
}
