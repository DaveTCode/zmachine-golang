package zobject

import (
	"fmt"

	"github.com/davetcode/goz/zcore"
)

type Property struct {
	Id                   uint8
	Length               uint8
	Data                 []uint8
	PropertyHeaderLength uint8
	Address              uint32
	DataAddress          uint32
}

// GetPropertyLength Property length is requested by the address of the first byte of the data
// This function therefore works back from that to find the property length
// based on the flags set on the property size byte(s)
func GetPropertyLength(core *zcore.Core, addr uint32) uint16 {
	if addr == 0 {
		return 0 // Special case required by some story files
	}

	prevByte := core.ReadByte(addr - 1)
	if core.Version <= 3 {
		return uint16(prevByte>>5) + 1
	} else if prevByte&0b1000_0000 != 0 {
		if prevByte&0b11_1111 == 0 {
			return 64 // Special case 0 length == 64
		}
		return uint16(prevByte & 0b11_1111)
	} else {
		return uint16(((prevByte >> 6) & 1) + 1)
	}
}

func (o *Object) SetProperty(propertyId uint8, value uint16, core *zcore.Core) error {
	objectNameLength := core.ReadByte(uint32(o.PropertyPointer))
	currentPtr := uint32(o.PropertyPointer + 1 + uint16(objectNameLength)*2)

	for {
		if core.ReadByte(currentPtr) == 0 {
			break
		}

		property := o.GetPropertyByAddress(currentPtr, core)

		if property.Id == propertyId {
			switch property.Length {
			case 1:
				core.WriteByte(currentPtr+1, uint8(value))
			case 2:
				core.WriteHalfWord(currentPtr+1, value)
			default:
				return fmt.Errorf("invalid property length %d, can't set value", property.Length)
			}

			return nil
		}

		currentPtr += uint32(property.Length) + uint32(property.PropertyHeaderLength)
	}

	// Property not found on object
	return fmt.Errorf("property %d not found on object %d", propertyId, o.Id)
}

func (o *Object) GetProperty(propertyId uint8, core *zcore.Core) Property {
	objectNameLength := core.ReadByte(uint32(o.PropertyPointer))
	currentPtr := uint32(o.PropertyPointer + 1 + uint16(objectNameLength)*2)

	for {
		// Property table ends with null terminator
		if core.ReadByte(currentPtr) == 0 {
			break
		}

		property := o.GetPropertyByAddress(currentPtr, core)

		if property.Id == propertyId {
			return property
		} // TODO can probably break here if properyId > property.Id since properties must be in descending order

		currentPtr += uint32(property.Length) + uint32(property.PropertyHeaderLength)
	}

	// Property not found on object, returning global default for that property
	propertyAddress := uint32(core.ObjectTableBase + 2*uint16(propertyId-1))
	return Property{
		Id:   propertyId,
		Data: core.ReadSlice(propertyAddress, propertyAddress+2),
	}
}

func (o *Object) GetPropertyByAddress(propertyAddr uint32, core *zcore.Core) Property {
	propertySizeByte := core.ReadByte(propertyAddr)
	length := (propertySizeByte >> 5) + 1
	id := propertySizeByte & 0b1_1111
	propertyHeaderLength := uint8(1)

	if core.Version >= 4 {
		if propertySizeByte>>7 == 1 {
			length = core.ReadByte(propertyAddr+1) & 0b11_1111

			// 12.4.2.1.1
			// [1.0] A value of 0 as property data length (in the second byte) should be interpreted as a length of 64. (Inform can compile such properties.)
			if length == 0 {
				length = 64
			}
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
		Data:                 core.ReadSlice(dataAddress, dataAddress+uint32(length)),
		PropertyHeaderLength: propertyHeaderLength,
		Address:              propertyAddr,
		DataAddress:          dataAddress,
	}
}

func (o *Object) GetNextProperty(propertyId uint8, core *zcore.Core) (uint8, error) {
	if propertyId == 0 { // Special case, means get first property
		if core.ReadByte(uint32(o.PropertyPointer)) == 0 {
			return 0, nil // Special case, no next property means return 0
		}

		objectNameLength := core.ReadByte(uint32(o.PropertyPointer))
		currentPtr := uint32(o.PropertyPointer + 1 + uint16(objectNameLength)*2)
		return o.GetPropertyByAddress(currentPtr, core).Id, nil
	}

	property := o.GetProperty(propertyId, core)
	if property.DataAddress == 0 {
		return 0, fmt.Errorf("invalid property id %d for object %d", propertyId, o.Id)
	}

	nextPropertyPtr := property.DataAddress + uint32(property.Length)
	return o.GetPropertyByAddress(nextPropertyPtr, core).Id, nil
}
