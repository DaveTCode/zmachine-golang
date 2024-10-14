package zobject_test

import (
	"os"
	"testing"

	"github.com/davetcode/goz/zmachine"
	"github.com/davetcode/goz/zobject"
	"github.com/davetcode/goz/zstring"
)

func TestZerothObjectRetrieval(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Retrieving object with id 0 should panic")
		}
	}()

	memory := []uint8{}

	zobject.GetObject(0, 0, memory, 1, zstring.LoadAlphabets(1, memory, 0))
}

func TestZork1V1ObjectRetrieval(t *testing.T) {
	romFileBytes, err := os.ReadFile("../zork1.z1")
	if err != nil {
		panic(err)
	}
	z := zmachine.LoadRom(romFileBytes, nil, nil, nil, nil)

	obj := zobject.GetObject(0x23, z.ObjectTableBase(), z.Memory, 1, z.Alphabets)

	if obj.Name != "West of House" {
		t.Errorf("Incorrect name %s", obj.Name)
	}
	if obj.Parent != 117 {
		t.Errorf("Incorrect parent %d", obj.Parent)
	}
	if obj.Child != 252 {
		t.Errorf("Incorrect child %d", obj.Child)
	}
	if obj.Sibling != 101 {
		t.Errorf("Incorrect sibling %d", obj.Sibling)
	}
	if obj.PropertyPointer != 0x0c79 {
		t.Errorf("Incorrect property pointer %x", obj.PropertyPointer)
	}
}

func TestZork1V1PropertyRetrieval(t *testing.T) {
	romFileBytes, err := os.ReadFile("../zork1.z1")
	if err != nil {
		panic(err)
	}
	z := zmachine.LoadRom(romFileBytes, nil, nil, nil, nil)

	obj := zobject.GetObject(1, z.ObjectTableBase(), z.Memory, 1, z.Alphabets) // Damp Cave

	// Length 1 property
	prop6 := obj.GetProperty(6, z.Memory, z.Version(), z.ObjectTableBase())
	if prop6.Length != 1 {
		t.Errorf("Incorrect property length %d", prop6.Length)
	}
	if prop6.Data[0] != 0x85 {
		t.Errorf("Incorrect property data %x", prop6.Data[0])
	}

	// Length 1 property
	prop11 := obj.GetProperty(11, z.Memory, z.Version(), z.ObjectTableBase())
	if prop11.Length != 2 {
		t.Errorf("Incorrect property length %d", prop11.Length)
	}
	if prop11.Data[0] != 0x88 || prop11.Data[1] != 0xe5 {
		t.Errorf("Incorrect property data %x%x", prop11.Data[0], prop11.Data[1])
	}

	// Non-existent property
	prop1 := obj.GetProperty(1, z.Memory, z.Version(), z.ObjectTableBase())
	if prop1.DataAddress != 0 {
		t.Error("Property 1 shouldn't exist on object 1")
	}

	// Non-existent property with value
	prop9 := obj.GetProperty(9, z.Memory, z.Version(), z.ObjectTableBase())
	if prop9.DataAddress != 0 {
		t.Error("Property 9 shouldn't exist on object 1")
	}
	if prop9.Data[0] != 0x00 || prop9.Data[1] != 0x05 {
		t.Errorf("Incorrect property data %x%x", prop9.Data[0], prop9.Data[1])
	}
}

func TestAttributesV1(t *testing.T) {
	romFileBytes, err := os.ReadFile("../zork1.z1")
	if err != nil {
		panic(err)
	}
	z := zmachine.LoadRom(romFileBytes, nil, nil, nil, nil)

	forest := zobject.GetObject(4, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets) // Forest

	if forest.TestAttribute(1) || forest.TestAttribute(4) || forest.TestAttribute(10) {
		t.Error("Forest should not have attributes 1,4,10 set")
	}
	if !(forest.TestAttribute(2) && forest.TestAttribute(3) && forest.TestAttribute(19)) {
		t.Error("Forest should have attributes 2,3,19 set")
	}

	forest.SetAttribute(10, z.Memory, z.Version())
	if !forest.TestAttribute(10) {
		t.Error("Setting attribute 10 didn't work")
	}

	forest.ClearAttribute(10, z.Memory, z.Version())
	if forest.TestAttribute(10) {
		t.Error("Clearing attribute 10 didn't work")
	}
}