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
	return zmachine.LoadRom(romFileBytes, nil, nil)
}

func TestZerothObjectRetrieval(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Retrieving object with id 0 should panic")
		}
	}()

	core := loadPraxix().Core

	zobject.GetObject(0, &core, zstring.LoadAlphabets(&core))
}

func TestZork1V1ObjectRetrieval(t *testing.T) {
	z := loadZork1()

	obj := zobject.GetObject(0x23, &z.Core, z.Alphabets)

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

	obj := zobject.GetObject(5, &z.Core, z.Alphabets) // Test Class

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

	obj := zobject.GetObject(1, &z.Core, z.Alphabets) // Damp Cave

	obj.SetProperty(11, 0xbeef, &z.Core)
	property := obj.GetProperty(11, &z.Core)
	if property.Data[0] != 0xbe || property.Data[1] != 0xef || property.Length != 2 {
		t.Error("Property set didn't work on existing same length property")
	}

	obj.SetProperty(6, 0xfeed, &z.Core)
	property = obj.GetProperty(6, &z.Core)
	if property.Data[0] != 0xed || property.Length != 1 {
		t.Error("Property set didn't work on short property")
	}
}

func TestZork1V1PropertyRetrieval(t *testing.T) {
	z := loadZork1()

	obj := zobject.GetObject(1, &z.Core, z.Alphabets) // Damp Cave

	// Length 1 property
	prop6 := obj.GetProperty(6, &z.Core)
	if prop6.Length != 1 {
		t.Errorf("Incorrect property length %d", prop6.Length)
	}
	if prop6.Data[0] != 0x85 {
		t.Errorf("Incorrect property data %x", prop6.Data[0])
	}

	if zobject.GetPropertyLength(&z.Core, prop6.DataAddress) != 1 {
		t.Error("Getting property length by address not working")
	}

	// Length 2 property
	prop11 := obj.GetProperty(11, &z.Core)
	if prop11.Length != 2 {
		t.Errorf("Incorrect property length %d", prop11.Length)
	}
	if prop11.Data[0] != 0x88 || prop11.Data[1] != 0xe5 {
		t.Errorf("Incorrect property data %x%x", prop11.Data[0], prop11.Data[1])
	}

	// Non-existent property
	prop1 := obj.GetProperty(1, &z.Core)
	if prop1.DataAddress != 0 {
		t.Error("Property 1 shouldn't exist on object 1")
	}

	// Non-existent property with value
	prop9 := obj.GetProperty(9, &z.Core)
	if prop9.DataAddress != 0 {
		t.Error("Property 9 shouldn't exist on object 1")
	}
	if prop9.Data[0] != 0x00 || prop9.Data[1] != 0x05 {
		t.Errorf("Incorrect property data %x%x", prop9.Data[0], prop9.Data[1])
	}
}

func TestPraxixV5Property(t *testing.T) {
	z := loadPraxix()

	obj := zobject.GetObject(6, &z.Core, z.Alphabets) // Test Class
	prop := obj.GetProperty(1, &z.Core)
	if prop.Length != 8 {
		t.Errorf("Incorrect property length %d", prop.Length)
	}
	if prop.Data[0] != 0x0e || prop.Data[7] != 0xf9 {
		t.Errorf("Incorrect property data 0x%x...%x", prop.Data[0], prop.Data[7])
	}

	propLength := zobject.GetPropertyLength(&z.Core, prop.DataAddress)
	if propLength != uint16(prop.Length) {
		t.Errorf("Getting property length from address doesn't match")
	}

	prop = obj.GetProperty(2, &z.Core)
	if prop.Length != 2 {
		t.Errorf("Incorrect property length %d", prop.Length)
	}
	if prop.Data[0] != 0x00 || prop.Data[1] != 0x05 {
		t.Errorf("Incorrect property data 0x%x...%x", prop.Data[0], prop.Data[1])
	}

	propLength = zobject.GetPropertyLength(&z.Core, prop.DataAddress)
	if propLength != uint16(prop.Length) {
		t.Errorf("Getting property length from address doesn't match")
	}

	prop = obj.GetProperty(3, &z.Core)
	if prop.Length != 2 {
		t.Errorf("Incorrect property length %d", prop.Length)
	}
	if prop.Data[0] != 0x06 || prop.Data[1] != 0x65 {
		t.Errorf("Incorrect property data 0x%x...%x", prop.Data[0], prop.Data[1])
	}

	propLength = zobject.GetPropertyLength(&z.Core, prop.DataAddress)
	if propLength != uint16(prop.Length) {
		t.Errorf("Getting property length from address doesn't match")
	}
}

func TestAttributesV1(t *testing.T) {
	z := loadZork1()

	forest := zobject.GetObject(4, &z.Core, z.Alphabets)

	if forest.TestAttribute(1) || forest.TestAttribute(4) || forest.TestAttribute(10) {
		t.Error("Forest should not have attributes 1,4,10 set")
	}
	if !(forest.TestAttribute(2) && forest.TestAttribute(3) && forest.TestAttribute(19)) {
		t.Error("Forest should have attributes 2,3,19 set")
	}

	forest.SetAttribute(10, &z.Core)
	if !forest.TestAttribute(10) {
		t.Error("Setting attribute 10 didn't work")
	}

	forest.ClearAttribute(10, &z.Core)
	if forest.TestAttribute(10) {
		t.Error("Clearing attribute 10 didn't work")
	}
}

func TestMoveObject(t *testing.T) {
	z := loadZork1()

	z.MoveObject(252, 4) // Move player to forest and then check

	westOfHouse := zobject.GetObject(35, &z.Core, z.Alphabets)
	cretin := zobject.GetObject(252, &z.Core, z.Alphabets)
	forest := zobject.GetObject(4, &z.Core, z.Alphabets)

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

// TestMoveObjectFirstChildToSameParent tests moving an object that is the first child
// of a parent back to the same parent. The object should remain first child with same sibling.
func TestMoveObjectFirstChildToSameParent(t *testing.T) {
	z := loadZork1()

	// In Zork1, object 35 (West of House) has child 252 (cretin/player) as first child
	westOfHouse := zobject.GetObject(35, &z.Core, z.Alphabets)
	originalFirstChild := westOfHouse.Child // 252 (cretin)
	originalSibling := zobject.GetObject(originalFirstChild, &z.Core, z.Alphabets).Sibling

	// Move the first child to the same parent - should re-insert as first child
	z.MoveObject(originalFirstChild, 35)

	// Re-read objects after move
	westOfHouse = zobject.GetObject(35, &z.Core, z.Alphabets)
	movedObj := zobject.GetObject(originalFirstChild, &z.Core, z.Alphabets)

	// Object should still be first child
	if westOfHouse.Child != originalFirstChild {
		t.Errorf("First child should still be %d, got %d", originalFirstChild, westOfHouse.Child)
	}
	// Sibling should be the same (was second child, still is)
	if movedObj.Sibling != originalSibling {
		t.Errorf("Object's sibling should still be %d, got %d", originalSibling, movedObj.Sibling)
	}

	// Check for circular sibling chain
	seen := make(map[uint16]bool)
	current := westOfHouse.Child
	for current != 0 {
		if seen[current] {
			t.Fatalf("Circular sibling chain detected! Object %d appears twice", current)
		}
		seen[current] = true
		current = zobject.GetObject(current, &z.Core, z.Alphabets).Sibling

		if len(seen) > 1000 {
			t.Fatal("Too many siblings - likely circular chain")
		}
	}
}

// TestMoveObjectNonFirstChildToSameParent tests moving an object that is NOT the first
// child of a parent back to the same parent. Per Z-machine spec, INSERT_OBJ should always
// make the object the FIRST child, so this should move it to the front.
func TestMoveObjectNonFirstChildToSameParent(t *testing.T) {
	z := loadZork1()

	// Get the children of West of House
	westOfHouse := zobject.GetObject(35, &z.Core, z.Alphabets)
	firstChild := westOfHouse.Child
	secondChild := zobject.GetObject(firstChild, &z.Core, z.Alphabets).Sibling

	if secondChild == 0 {
		t.Skip("Need an object with at least 2 children for this test")
	}

	thirdChild := zobject.GetObject(secondChild, &z.Core, z.Alphabets).Sibling

	// Move the second child to the same parent - should become the first child
	z.MoveObject(secondChild, 35)

	// Re-read objects after move
	westOfHouse = zobject.GetObject(35, &z.Core, z.Alphabets)
	movedObj := zobject.GetObject(secondChild, &z.Core, z.Alphabets)
	originalFirstChild := zobject.GetObject(firstChild, &z.Core, z.Alphabets)

	// The moved object should now be the first child
	if westOfHouse.Child != secondChild {
		t.Errorf("Moved object should now be first child, got %d", westOfHouse.Child)
	}

	// The moved object's sibling should be the original first child
	if movedObj.Sibling != firstChild {
		t.Errorf("Moved object's sibling should be %d, got %d", firstChild, movedObj.Sibling)
	}

	// The original first child's sibling should now be the third child (skipping the moved one)
	if originalFirstChild.Sibling != thirdChild {
		t.Errorf("Original first child's sibling should be %d, got %d", thirdChild, originalFirstChild.Sibling)
	}

	// Check for circular sibling chain
	seen := make(map[uint16]bool)
	current := westOfHouse.Child
	for current != 0 {
		if seen[current] {
			t.Fatalf("Circular sibling chain detected! Object %d appears twice", current)
		}
		seen[current] = true
		current = zobject.GetObject(current, &z.Core, z.Alphabets).Sibling

		if len(seen) > 1000 {
			t.Fatal("Too many siblings - likely circular chain")
		}
	}
}

// TestMoveObjectToDifferentParentThenBack tests the scenario that caused the original bug:
// moving an object away and then back to its original parent. The second move must
// correctly read the destination's child pointer AFTER unlinking.
func TestMoveObjectToDifferentParentThenBack(t *testing.T) {
	z := loadZork1()

	// Get initial state - West of House (35) has cretin (252) as first child
	westOfHouse := zobject.GetObject(35, &z.Core, z.Alphabets)
	cretin := westOfHouse.Child // 252
	cretinOriginalSibling := zobject.GetObject(cretin, &z.Core, z.Alphabets).Sibling

	// Move cretin to forest (4)
	z.MoveObject(cretin, 4)

	// Verify cretin is now in forest
	forest := zobject.GetObject(4, &z.Core, z.Alphabets)
	if forest.Child != cretin {
		t.Fatalf("Cretin should be first child of forest after first move")
	}

	// Now move cretin back to West of House - this is where the bug would manifest
	z.MoveObject(cretin, 35)

	// Re-read all objects
	westOfHouse = zobject.GetObject(35, &z.Core, z.Alphabets)
	cretinObj := zobject.GetObject(cretin, &z.Core, z.Alphabets)

	// Cretin should now be first child of West of House
	if westOfHouse.Child != cretin {
		t.Errorf("Cretin should be first child of West of House, got %d", westOfHouse.Child)
	}

	// Cretin's sibling should be what was previously the first child (after cretin left)
	if cretinObj.Sibling != cretinOriginalSibling {
		t.Errorf("Cretin's sibling should be %d, got %d", cretinOriginalSibling, cretinObj.Sibling)
	}

	// Critical: check for circular sibling chain
	seen := make(map[uint16]bool)
	current := westOfHouse.Child
	for current != 0 {
		if seen[current] {
			t.Fatalf("Circular sibling chain detected! Object %d appears twice", current)
		}
		seen[current] = true
		current = zobject.GetObject(current, &z.Core, z.Alphabets).Sibling

		if len(seen) > 1000 {
			t.Fatal("Too many siblings - likely circular chain")
		}
	}
}

func TestGetNextPropertyV1(t *testing.T) {
	z := loadZork1()

	dampCave := zobject.GetObject(1, &z.Core, z.Alphabets)
	noNameNoProps := zobject.GetObject(117, &z.Core, z.Alphabets)

	firstProp := dampCave.GetNextProperty(0, &z.Core)
	if firstProp != 30 {
		t.Fatalf("First property of damp cave should have been 30")
	}

	nextProp := dampCave.GetNextProperty(28, &z.Core)
	if nextProp != 11 {
		t.Fatalf("Next property of damp cave after 28 should have been 11")
	}

	afterLastProp := dampCave.GetNextProperty(6, &z.Core)
	if afterLastProp != 0 {
		t.Fatalf("Should be no property after 6 on damp cave object")
	}

	if noNameNoProps.GetNextProperty(0, &z.Core) != 0 {
		t.Fatalf("Object with no property should always return 0 even for first prop")
	}
}
