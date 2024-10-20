package zobject_test

import (
	"os"
	"testing"

	"github.com/davetcode/goz/zmachine"
	"github.com/davetcode/goz/zobject"
	"github.com/davetcode/goz/zstring"
)

func loadZork1() *zmachine.ZMachine  { return loadRom("../zork1.z1") }
func loadPraxix() *zmachine.ZMachine { return loadRom("../praxix.z5") }

func loadRom(file string) *zmachine.ZMachine {
	romFileBytes, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}
	return zmachine.LoadRom(romFileBytes, nil, nil, nil, nil)
}

func TestZerothObjectRetrieval(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Retrieving object with id 0 should panic")
		}
	}()

	memory := []uint8{}

	zobject.GetObject(0, 0, memory, 1, zstring.LoadAlphabets(1, memory, 0), 0)
}

func TestZork1V1ObjectRetrieval(t *testing.T) {
	z := loadZork1()

	obj := zobject.GetObject(0x23, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())

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

func TestPraxixV5ObjectRetrieval(t *testing.T) {
	z := loadPraxix()

	obj := zobject.GetObject(5, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()) // Test Class

	if obj.Name != "TestClass" {
		t.Errorf("Incorrect name %s", obj.Name)
	}
	if obj.Parent != 1 {
		t.Errorf("Incorrect parent %d", obj.Parent)
	}
	if obj.Child != 0 {
		t.Errorf("Incorrect child %d", obj.Child)
	}
	if obj.Sibling != 0 {
		t.Errorf("Incorrect sibling %d", obj.Sibling)
	}
	if obj.PropertyPointer != 0x032c {
		t.Errorf("Incorrect property pointer %x", obj.PropertyPointer)
	}
}

func TestSetPropertyV1(t *testing.T) {
	z := loadZork1()

	obj := zobject.GetObject(1, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()) // Damp Cave

	obj.SetProperty(11, 0xbeef, z.Memory, z.Version(), z.ObjectTableBase())
	property := obj.GetProperty(11, z.Memory, z.Version(), z.ObjectTableBase())
	if property.Data[0] != 0xbe || property.Data[1] != 0xef || property.Length != 2 {
		t.Error("Property set didn't work on existing same length property")
	}

	obj.SetProperty(6, 0xfeed, z.Memory, z.Version(), z.ObjectTableBase())
	property = obj.GetProperty(6, z.Memory, z.Version(), z.ObjectTableBase())
	if property.Data[0] != 0xed || property.Length != 1 {
		t.Error("Property set didn't work on short property")
	}
}

func TestZork1V1PropertyRetrieval(t *testing.T) {
	z := loadZork1()

	obj := zobject.GetObject(1, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()) // Damp Cave

	// Length 1 property
	prop6 := obj.GetProperty(6, z.Memory, z.Version(), z.ObjectTableBase())
	if prop6.Length != 1 {
		t.Errorf("Incorrect property length %d", prop6.Length)
	}
	if prop6.Data[0] != 0x85 {
		t.Errorf("Incorrect property data %x", prop6.Data[0])
	}

	if zobject.GetPropertyLength(z.Memory, prop6.DataAddress, z.Version()) != 1 {
		t.Error("Getting property length by address not working")
	}

	// Length 2 property
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

func TestPraxixV5Property(t *testing.T) {
	z := loadPraxix()

	obj := zobject.GetObject(6, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()) // Test Class
	prop := obj.GetProperty(1, z.Memory, z.Version(), z.ObjectTableBase())
	if prop.Length != 8 {
		t.Errorf("Incorrect property length %d", prop.Length)
	}
	if prop.Data[0] != 0x0e || prop.Data[7] != 0xf9 {
		t.Errorf("Incorrect property data 0x%x...%x", prop.Data[0], prop.Data[7])
	}

	propLength := zobject.GetPropertyLength(z.Memory, prop.DataAddress, z.Version())
	if propLength != uint16(prop.Length) {
		t.Errorf("Getting property length from address doesn't match")
	}

	prop = obj.GetProperty(2, z.Memory, z.Version(), z.ObjectTableBase())
	if prop.Length != 2 {
		t.Errorf("Incorrect property length %d", prop.Length)
	}
	if prop.Data[0] != 0x00 || prop.Data[1] != 0x05 {
		t.Errorf("Incorrect property data 0x%x...%x", prop.Data[0], prop.Data[1])
	}

	propLength = zobject.GetPropertyLength(z.Memory, prop.DataAddress, z.Version())
	if propLength != uint16(prop.Length) {
		t.Errorf("Getting property length from address doesn't match")
	}

	prop = obj.GetProperty(3, z.Memory, z.Version(), z.ObjectTableBase())
	if prop.Length != 2 {
		t.Errorf("Incorrect property length %d", prop.Length)
	}
	if prop.Data[0] != 0x06 || prop.Data[1] != 0x65 {
		t.Errorf("Incorrect property data 0x%x...%x", prop.Data[0], prop.Data[1])
	}

	propLength = zobject.GetPropertyLength(z.Memory, prop.DataAddress, z.Version())
	if propLength != uint16(prop.Length) {
		t.Errorf("Getting property length from address doesn't match")
	}
}

func TestAttributesV1(t *testing.T) {
	z := loadZork1()

	forest := zobject.GetObject(4, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase()) // Forest

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

func TestMoveObject(t *testing.T) {
	z := loadZork1()

	z.MoveObject(252, 4) // Move player to forest and then check

	westOfHouse := zobject.GetObject(35, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
	cretin := zobject.GetObject(252, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
	forest := zobject.GetObject(4, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())

	if westOfHouse.Child != 199 { // mailbox
		t.Error("West of house should now have mailbox as first child")
	}
	if cretin.Parent != forest.Id {
		t.Error("Player should now have parent set to forest")
	}
	if forest.Child != cretin.Id {
		t.Error("Forest should now have child set to cretin")
	}
	if cretin.Sibling != 0 {
		t.Error("Cretin should now have no sibling")
	}
}

func TestGetNextPropertyV1(t *testing.T) {
	z := loadZork1()

	dampCave := zobject.GetObject(1, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())
	noNameNoProps := zobject.GetObject(117, z.ObjectTableBase(), z.Memory, z.Version(), z.Alphabets, z.AbbreviationTableBase())

	firstProp := dampCave.GetNextProperty(0, z.Memory, z.Version(), z.ObjectTableBase())
	if firstProp != 30 {
		t.Fatalf("First property of damp cave should have been 30")
	}

	nextProp := dampCave.GetNextProperty(28, z.Memory, z.Version(), z.ObjectTableBase())
	if nextProp != 11 {
		t.Fatalf("Next property of damp cave after 28 should have been 11")
	}

	afterLastProp := dampCave.GetNextProperty(6, z.Memory, z.Version(), z.ObjectTableBase())
	if afterLastProp != 0 {
		t.Fatalf("Should be no property after 6 on damp cave object")
	}

	if noNameNoProps.GetNextProperty(0, z.Memory, z.Version(), z.ObjectTableBase()) != 0 {
		t.Fatalf("Object with no property should always return 0 even for first prop")
	}
}
