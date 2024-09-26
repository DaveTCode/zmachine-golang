package main

import (
	"encoding/binary"
	"fmt"
)

type Object struct {
	id              uint16
	attributes      uint64 // Bytes 0-3 are valid in all versions, 4-5 are only populated in V4+
	parent          uint16 // uint8 on v1-3
	sibling         uint16 // uint8 on v1-3
	child           uint16 // uint8 on v1-3
	propertyPointer uint16
}

func (z *ZMachine) setObjectProperty(objId uint16, propertyId uint8, value uint16) {
	object := z.getObject(objId)
	objectNameLength := z.readByte(object.propertyPointer)
	currentPtr := object.propertyPointer + 1 + uint16(objectNameLength)*2

	for {
		// TODO - Handle v4+ properties
		propertySizeByte := z.readByte(uint16(currentPtr))

		if propertySizeByte == 0 {
			break
		}

		currentPropertyId := propertySizeByte & 0b1_1111
		propertySize := (propertySizeByte >> 5) + 1

		if currentPropertyId == propertyId {
			switch propertySize {
			case 1:
				z.writeByte(currentPtr+1, uint8(value))
			case 2:
				z.writeHalfWord(currentPtr+1, value)
			default:
				panic(fmt.Sprintf("Invalid property length %d, can't set value", propertyId))
			}

			return
		}

		currentPtr += uint16(propertySize) + 1
	}

	// Property not found on object, returning global default for that property
	panic(fmt.Sprintf("Invalid property (%d) requested for object (%d)", propertyId, objId))
}

func (z *ZMachine) getObjectProperty(objId uint16, propertyId uint8) []uint8 {
	object := z.getObject(objId)
	objectNameLength := z.readByte(object.propertyPointer)
	currentPtr := object.propertyPointer + 1 + uint16(objectNameLength)*2

	for {
		// TODO - Handle v4+ properties
		propertySizeByte := z.readByte(uint16(currentPtr))

		if propertySizeByte == 0 {
			break
		}

		currentPropertyId := propertySizeByte & 0b1_1111
		propertySize := (propertySizeByte >> 5) + 1

		if currentPropertyId == propertyId {
			return z.memory[currentPtr+1 : currentPtr+1+uint16(propertySize)]
		}

		currentPtr += uint16(propertySize) + 1
	}

	// Property not found on object, returning global default for that property
	defaultTableBase := z.objectTableBase()
	propertyAddress := defaultTableBase + 2*uint16(propertyId)
	return z.memory[propertyAddress : propertyAddress+2]
}

func (z *ZMachine) getObjectName(objId uint16) string {
	obj := z.getObject(objId)

	name, _ := z.readZString(obj.propertyPointer + 1)

	return name
}

func (z *ZMachine) getObject(i uint16) Object {
	if i == 0 {
		panic("Can't get 0th object, it doesn't exist")
	} else if i == 199 {
		i = 199
	}
	base := z.objectTableBase()

	if z.version() >= 4 {
		objectBase := base + 63*2 + (i-1)*14

		return Object{
			id:              i,
			attributes:      (binary.BigEndian.Uint64(z.memory[objectBase:objectBase+8]) >> 16) << 16,
			parent:          z.readHalfWord(objectBase + 6),
			sibling:         z.readHalfWord(objectBase + 8),
			child:           z.readHalfWord(objectBase + 10),
			propertyPointer: z.readHalfWord(objectBase + 12),
		}
	} else {
		objectBase := base + 31*2 + (i-1)*9

		return Object{
			id:              i,
			attributes:      (binary.BigEndian.Uint64(z.memory[objectBase:objectBase+8]) >> 32) << 32,
			parent:          uint16(z.memory[objectBase+4]),
			sibling:         uint16(z.memory[objectBase+5]),
			child:           uint16(z.memory[objectBase+6]),
			propertyPointer: z.readHalfWord(objectBase + 7),
		}
	}
}
