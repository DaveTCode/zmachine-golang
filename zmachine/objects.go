package zmachine

import (
	"encoding/binary"
	"fmt"

	"github.com/davetcode/goz/zstring"
)

type Object struct {
	baseAddress     uint32
	id              uint16
	attributes      uint64 // Bytes 0-3 are valid in all versions, 4-5 are only populated in V4+
	parent          uint16 // uint8 on v1-3
	sibling         uint16 // uint8 on v1-3
	child           uint16 // uint8 on v1-3
	propertyPointer uint16
}

type Property struct {
	id                   uint8
	length               uint8
	data                 []uint8
	propertyHeaderLength uint8
	address              uint16
}

func (z *ZMachine) moveObject(objId uint16, newParent uint16) {
	object := z.getObject(objId)
	destinationObject := z.getObject(newParent)
	oldParent := z.getObject(object.parent)

	// Remove from old location in the sibling chain
	if oldParent.child == object.id {
		// First child case
		oldParent.setChild(object.sibling, z.version(), z.memory)
	} else {
		// Non-first child case
		currObjId := uint16(oldParent.child)
		for {
			if currObjId == 0 {
				break
			}

			currObj := z.getObject(currObjId)
			if currObj.sibling == object.id {
				currObj.setSibling(object.sibling, z.version(), z.memory)
				break
			}
		}
	}

	// Set new location in the tree
	object.setSibling(destinationObject.child, z.version(), z.memory)
	object.setParent(destinationObject.id, z.version(), z.memory)
	destinationObject.setChild(object.id, z.version(), z.memory)
}

func (z *ZMachine) setObjectProperty(objId uint16, propertyId uint8, value uint16) {
	object := z.getObject(objId)
	objectNameLength := z.readByte(uint32(object.propertyPointer))
	currentPtr := uint32(object.propertyPointer + 1 + uint16(objectNameLength)*2)

	for {
		if z.readByte(currentPtr) == 0 {
			break
		}

		property := z.getObjectProperty(objId, propertyId)

		if property.id == propertyId {
			switch property.length {
			case 1:
				z.writeByte(currentPtr+1, uint8(value))
			case 2:
				z.writeHalfWord(currentPtr+1, value)
			default:
				panic(fmt.Sprintf("Invalid property length %d, can't set value", propertyId))
			}

			return
		}

		currentPtr += uint32(property.length) + uint32(property.propertyHeaderLength)
	}

	// Property not found on object, returning global default for that property
	panic(fmt.Sprintf("Invalid property (%d) requested for object (%d)", propertyId, objId))
}

func (z *ZMachine) getObjectProperty(objId uint16, propertyId uint8) Property {
	object := z.getObject(objId)
	objectNameLength := z.readByte(uint32(object.propertyPointer))
	currentPtr := uint32(object.propertyPointer + 1 + uint16(objectNameLength)*2)

	for {
		// Property table ends with null terminator
		if z.readByte(currentPtr) == 0 {
			break
		}

		property := z.getPropertyByAddress(currentPtr)

		if property.id == propertyId {
			return property
		}

		currentPtr += uint32(property.length) + uint32(property.propertyHeaderLength)
	}

	// Property not found on object, returning global default for that property
	defaultTableBase := z.objectTableBase()
	propertyAddress := defaultTableBase + 2*uint16(propertyId)
	return Property{
		id:   propertyId,
		data: z.memory[propertyAddress : propertyAddress+2],
	}
}

func (z *ZMachine) getPropertyByAddress(propertyAddr uint32) Property {
	propertySizeByte := z.readByte(propertyAddr)
	length := (propertySizeByte >> 5) + 1
	id := propertySizeByte & 0b1_1111
	data := z.memory[propertyAddr+1 : propertyAddr+1+uint32(length)]
	propertyHeaderLength := uint8(1)

	if z.version() >= 4 {
		if propertySizeByte>>7 == 1 {
			length = (z.readByte(propertyAddr+1) & 0b11_1111) + 1
			id = propertySizeByte & 0b11_1111
			data = z.memory[propertyAddr+2 : propertyAddr+2+uint32(length)]
			propertyHeaderLength = 2
		} else {
			length = ((propertySizeByte >> 6) & 1) + 1
			id = propertySizeByte & 0b11_1111
		}
	}

	return Property{
		id:                   id,
		length:               length,
		data:                 data,
		propertyHeaderLength: propertyHeaderLength,
		address:              uint16(propertyAddr),
	}
}

func (z *ZMachine) getObjectName(objId uint16) string {
	obj := z.getObject(objId)

	name, _ := zstring.ReadZString(z.memory[obj.propertyPointer+1:], z.version())

	return name
}

func (z *ZMachine) getObject(i uint16) Object {
	if i == 0 {
		panic("Can't get 0th object, it doesn't exist")
	}

	base := z.objectTableBase()

	if z.version() >= 4 {
		objectBase := uint32(base + 63*2 + (i-1)*14)

		return Object{
			id:              i,
			attributes:      (binary.BigEndian.Uint64(z.memory[objectBase:objectBase+8]) >> 16) << 16,
			parent:          z.readHalfWord(objectBase + 6),
			sibling:         z.readHalfWord(objectBase + 8),
			child:           z.readHalfWord(objectBase + 10),
			propertyPointer: z.readHalfWord(objectBase + 12),
			baseAddress:     objectBase,
		}
	} else {
		objectBase := uint32(base + 31*2 + (i-1)*9)

		return Object{
			id:              i,
			attributes:      (binary.BigEndian.Uint64(z.memory[objectBase:objectBase+8]) >> 32) << 32,
			parent:          uint16(z.memory[objectBase+4]),
			sibling:         uint16(z.memory[objectBase+5]),
			child:           uint16(z.memory[objectBase+6]),
			propertyPointer: z.readHalfWord(objectBase + 7),
			baseAddress:     objectBase,
		}
	}
}

func (o *Object) setParent(parent uint16, version uint8, memory []uint8) {
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.baseAddress+6:o.baseAddress+8], parent)
	} else {
		memory[o.baseAddress+4] = uint8(parent)
	}
	o.parent = parent
}

func (o *Object) setSibling(sibling uint16, version uint8, memory []uint8) {
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.baseAddress+8:o.baseAddress+10], sibling)
	} else {
		memory[o.baseAddress+5] = uint8(sibling)
	}
	o.sibling = sibling
}

func (o *Object) setChild(child uint16, version uint8, memory []uint8) {
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.baseAddress+10:o.baseAddress+12], child)
	} else {
		memory[o.baseAddress+6] = uint8(child)
	}
	o.child = child
}
