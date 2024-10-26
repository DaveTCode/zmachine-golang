package zobject

import (
	"encoding/binary"

	"github.com/davetcode/goz/zstring"
)

type Object struct {
	BaseAddress     uint32
	Id              uint16
	Name            string
	Attributes      uint64 // Bytes 0-3 are valid in all versions, 4-5 are only populated in V4+
	Parent          uint16 // uint8 on v1-3
	Sibling         uint16 // uint8 on v1-3
	Child           uint16 // uint8 on v1-3
	PropertyPointer uint16
}

func GetObject(objId uint16, objectTableBase uint16, memory []uint8, version uint8, alphabets *zstring.Alphabets, AbbreviationTableBase uint16) Object {
	if objId == 0 {
		panic("Can't get 0th object, it doesn't exist")
	}

	if version >= 4 {
		objectBase := uint32(objectTableBase + 63*2 + (objId-1)*14)
		propertyPtr := binary.BigEndian.Uint16(memory[objectBase+12 : objectBase+14])
		nameLength := memory[propertyPtr]
		name, _ := zstring.Decode(memory, uint32(propertyPtr+1), uint32(propertyPtr+1+uint16(nameLength)*2), version, alphabets, AbbreviationTableBase, false)

		return Object{
			Id:              objId,
			Name:            name,
			Attributes:      (binary.BigEndian.Uint64(memory[objectBase:objectBase+8]) >> 16) << 16,
			Parent:          binary.BigEndian.Uint16(memory[objectBase+6 : objectBase+8]),
			Sibling:         binary.BigEndian.Uint16(memory[objectBase+8 : objectBase+10]),
			Child:           binary.BigEndian.Uint16(memory[objectBase+10 : objectBase+12]),
			PropertyPointer: propertyPtr,
			BaseAddress:     objectBase,
		}
	} else {
		objectBase := uint32(objectTableBase + 31*2 + (objId-1)*9)
		propertyPtr := binary.BigEndian.Uint16(memory[objectBase+7 : objectBase+9])
		nameLength := memory[propertyPtr]
		name, _ := zstring.Decode(memory, uint32(propertyPtr+1), uint32(propertyPtr+1+uint16(nameLength)*2), version, alphabets, AbbreviationTableBase, false)

		return Object{
			Id:              objId,
			Name:            name,
			Attributes:      (binary.BigEndian.Uint64(memory[objectBase:objectBase+8]) >> 32) << 32,
			Parent:          uint16(memory[objectBase+4]),
			Sibling:         uint16(memory[objectBase+5]),
			Child:           uint16(memory[objectBase+6]),
			PropertyPointer: propertyPtr,
			BaseAddress:     objectBase,
		}
	}
}

func (o *Object) TestAttribute(attribute uint16) bool {
	mask := uint64(1) << (63 - attribute)

	return (o.Attributes & mask) == mask
}

func (o *Object) SetAttribute(attribute uint16, memory []uint8, version uint8) {
	mask := uint64(1) << (63 - attribute)
	o.Attributes |= mask

	binary.BigEndian.PutUint32(memory[o.BaseAddress:o.BaseAddress+4], uint32(o.Attributes>>32))
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.BaseAddress+4:o.BaseAddress+6], uint16(o.Attributes>>16))
	}
}

func (o *Object) ClearAttribute(attribute uint16, memory []uint8, version uint8) {
	mask := uint64(1) << (63 - attribute)
	o.Attributes &= ^mask

	binary.BigEndian.PutUint32(memory[o.BaseAddress:o.BaseAddress+4], uint32(o.Attributes>>32))
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.BaseAddress+4:o.BaseAddress+6], uint16(o.Attributes>>16))
	}
}

func (o *Object) SetParent(parent uint16, version uint8, memory []uint8) {
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.BaseAddress+6:o.BaseAddress+8], parent)
	} else {
		memory[o.BaseAddress+4] = uint8(parent)
	}
	o.Parent = parent
}

func (o *Object) SetSibling(sibling uint16, version uint8, memory []uint8) {
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.BaseAddress+8:o.BaseAddress+10], sibling)
	} else {
		memory[o.BaseAddress+5] = uint8(sibling)
	}
	o.Sibling = sibling
}

func (o *Object) SetChild(child uint16, version uint8, memory []uint8) {
	if version >= 4 {
		binary.BigEndian.PutUint16(memory[o.BaseAddress+10:o.BaseAddress+12], child)
	} else {
		memory[o.BaseAddress+6] = uint8(child)
	}
	o.Child = child
}
