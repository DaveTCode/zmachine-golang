package zobject

import (
	"fmt"

	"github.com/davetcode/goz/zcore"
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

// NullObject represents object 0 (nothing). It is used when code needs to handle
// object 0 gracefully instead of panicking.
var NullObject = Object{
	Id:              0,
	Name:            "",
	Attributes:      0,
	Parent:          0,
	Sibling:         0,
	Child:           0,
	PropertyPointer: 0,
	BaseAddress:     0,
}

// GetObjectSafe returns NullObject for object 0 instead of panicking.
// Use this when the calling opcode should handle object 0 gracefully.
func GetObjectSafe(objId uint16, core *zcore.Core, alphabets *zstring.Alphabets) Object {
	if objId == 0 {
		return NullObject
	}
	return GetObject(objId, core, alphabets)
}

func GetObject(objId uint16, core *zcore.Core, alphabets *zstring.Alphabets) Object {
	if objId == 0 {
		panic(fmt.Sprintf("Can't get 0th object, it doesn't exist (version=%d, objectTableBase=0x%x)", 
			core.Version, core.ObjectTableBase))
	}

	if core.Version >= 4 {
		objectBase := uint32(core.ObjectTableBase + 63*2 + (objId-1)*14)
		propertyPtr := core.ReadHalfWord(objectBase + 12)
		nameLength := core.ReadByte(uint32(propertyPtr))
		name, _ := zstring.Decode(uint32(propertyPtr+1), uint32(propertyPtr+1+uint16(nameLength)*2), core, alphabets, false)

		return Object{
			Id:              objId,
			Name:            name,
			Attributes:      (core.ReadLongWord(objectBase) >> 16) << 16,
			Parent:          core.ReadHalfWord(objectBase + 6),
			Sibling:         core.ReadHalfWord(objectBase + 8),
			Child:           core.ReadHalfWord(objectBase + 10),
			PropertyPointer: propertyPtr,
			BaseAddress:     objectBase,
		}
	} else {
		objectBase := uint32(core.ObjectTableBase + 31*2 + (objId-1)*9)
		propertyPtr := core.ReadHalfWord(objectBase + 7)
		nameLength := core.ReadByte(uint32(propertyPtr))
		name, _ := zstring.Decode(uint32(propertyPtr+1), uint32(propertyPtr+1+uint16(nameLength)*2), core, alphabets, false)

		return Object{
			Id:              objId,
			Name:            name,
			Attributes:      (core.ReadLongWord(objectBase) >> 32) << 32,
			Parent:          uint16(core.ReadByte(objectBase + 4)),
			Sibling:         uint16(core.ReadByte(objectBase + 5)),
			Child:           uint16(core.ReadByte(objectBase + 6)),
			PropertyPointer: propertyPtr,
			BaseAddress:     objectBase,
		}
	}
}

func (o *Object) TestAttribute(attribute uint16) bool {
	mask := uint64(1) << (63 - attribute)

	return (o.Attributes & mask) == mask
}

func (o *Object) SetAttribute(attribute uint16, core *zcore.Core) {
	mask := uint64(1) << (63 - attribute)
	o.Attributes |= mask

	core.WriteWord(o.BaseAddress, uint32(o.Attributes>>32))
	if core.Version >= 4 {
		core.WriteHalfWord(o.BaseAddress+4, uint16(o.Attributes>>16))
	}
}

func (o *Object) ClearAttribute(attribute uint16, core *zcore.Core) {
	mask := uint64(1) << (63 - attribute)
	o.Attributes &= ^mask

	core.WriteWord(o.BaseAddress, uint32(o.Attributes>>32))
	if core.Version >= 4 {
		core.WriteHalfWord(o.BaseAddress+4, uint16(o.Attributes>>16))
	}
}

func (o *Object) SetParent(parent uint16, core *zcore.Core) {
	if core.Version >= 4 {
		core.WriteHalfWord(o.BaseAddress+6, parent)
	} else {
		core.WriteByte(o.BaseAddress+4, uint8(parent))
	}
	o.Parent = parent
}

func (o *Object) SetSibling(sibling uint16, core *zcore.Core) {
	if core.Version >= 4 {
		core.WriteHalfWord(o.BaseAddress+8, sibling)
	} else {
		core.WriteByte(o.BaseAddress+5, uint8(sibling))
	}
	o.Sibling = sibling
}

func (o *Object) SetChild(child uint16, core *zcore.Core) {
	if core.Version >= 4 {
		core.WriteHalfWord(o.BaseAddress+10, child)
	} else {
		core.WriteByte(o.BaseAddress+6, uint8(child))
	}
	o.Child = child
}
