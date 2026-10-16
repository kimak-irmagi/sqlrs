package runtimev2_test

import (
	"bytes"
	"reflect"
	"strings"
	"sync"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// ES20: inspection adds immutable views, not a second canonical encoding.
// Requirements: docs/architecture/runtime-v2-execution-schema-tests.md.
func TestCanonicalValueInspectionKindsAndOwnership(t *testing.T) {
	a := inspectString(t, "a")
	b := inspectString(t, "b")
	list, err := runtimev2.CanonicalList([]runtimev2.CanonicalValue{b, a})
	if err != nil {
		t.Fatal(err)
	}
	set, err := runtimev2.CanonicalSet([]runtimev2.CanonicalValue{b, a})
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := runtimev2.CanonicalMap([]runtimev2.CanonicalMapEntry{{Key: "b", Value: b}, {Key: "a", Value: a}})
	if err != nil {
		t.Fatal(err)
	}
	emptyList, _ := runtimev2.CanonicalList(nil)
	emptySet, _ := runtimev2.CanonicalSet(nil)
	emptyMap, _ := runtimev2.CanonicalMap(nil)
	cases := []struct {
		name                            string
		value                           runtimev2.CanonicalValue
		kind                            runtimev2.CanonicalValueKind
		text                            string
		stringOK, elementsOK, entriesOK bool
	}{
		{"zero", runtimev2.CanonicalValue{}, "", "", false, false, false},
		{"null", runtimev2.CanonicalNull(), runtimev2.CanonicalValueKindNull, "", false, false, false},
		{"string", a, runtimev2.CanonicalValueKindString, "a", true, false, false},
		{"empty string", inspectString(t, ""), runtimev2.CanonicalValueKindString, "", true, false, false},
		{"list", list, runtimev2.CanonicalValueKindList, "", false, true, false},
		{"set", set, runtimev2.CanonicalValueKindSet, "", false, true, false},
		{"map", mapping, runtimev2.CanonicalValueKindMap, "", false, false, true},
		{"empty list", emptyList, runtimev2.CanonicalValueKindList, "", false, true, false},
		{"empty set", emptySet, runtimev2.CanonicalValueKindSet, "", false, true, false},
		{"empty map", emptyMap, runtimev2.CanonicalValueKindMap, "", false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, token, depth, nodes := tc.value.CanonicalBytes(), tc.value.Token(), tc.value.Depth(), tc.value.NodeCount()
			if got := tc.value.Kind(); got != tc.kind {
				t.Fatalf("kind=%q, want %q", got, tc.kind)
			}
			if text, ok := tc.value.StringValue(); text != tc.text || ok != tc.stringOK {
				t.Fatalf("string=%q/%v", text, ok)
			}
			elements, ok := tc.value.Elements()
			if ok != tc.elementsOK || (ok && elements == nil) || (!ok && elements != nil) {
				t.Fatalf("elements=%v/%v", elements, ok)
			}
			entries, ok := tc.value.Entries()
			if ok != tc.entriesOK || (ok && entries == nil) || (!ok && entries != nil) {
				t.Fatalf("entries=%v/%v", entries, ok)
			}
			if tc.value.EncodedSize() != len(before) {
				t.Fatal("encoded size differs from canonical bytes")
			}
			if len(elements) != 0 {
				elements[0] = runtimev2.CanonicalValue{}
			}
			if len(entries) != 0 {
				entries[0].Key = "changed"
				entries[0].Value = runtimev2.CanonicalValue{}
			}
			if !bytes.Equal(before, tc.value.CanonicalBytes()) || token != tc.value.Token() || depth != tc.value.Depth() || nodes != tc.value.NodeCount() {
				t.Fatal("inspection mutated canonical value")
			}
		})
	}
	listItems, _ := list.Elements()
	setItems, _ := set.Elements()
	mapItems, _ := mapping.Entries()
	if !reflect.DeepEqual(inspectTexts(t, listItems), []string{"b", "a"}) || !reflect.DeepEqual(inspectTexts(t, setItems), []string{"a", "b"}) || mapItems[0].Key != "a" || mapItems[1].Key != "b" {
		t.Fatal("collection order or defensive slice changed")
	}
	childBytes := mapItems[0].Value.CanonicalBytes()
	childBytes[0] ^= 1
	if text, ok := mapItems[0].Value.StringValue(); !ok || text != "a" {
		t.Fatal("child bytes are aliased")
	}
}

var inspectSizeSink int

// ES20 requires no copy/allocation; len(CanonicalBytes()) is not sufficient.
func TestCanonicalValueEncodedSizeDoesNotAllocate(t *testing.T) {
	largeText := inspectString(t, strings.Repeat("x", runtimev2.MaxCanonicalStringBytes))
	children := make([]runtimev2.CanonicalValue, 64)
	for index := range children {
		children[index] = largeText
	}
	large, err := runtimev2.CanonicalList(children)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []runtimev2.CanonicalValue{{}, runtimev2.CanonicalNull(), inspectString(t, "small"), large} {
		if allocations := testing.AllocsPerRun(100, func() { inspectSizeSink = value.EncodedSize() }); allocations != 0 {
			t.Fatalf("EncodedSize allocated %v times", allocations)
		}
	}
}

func TestCanonicalValueInspectionConcurrentReads(t *testing.T) {
	value, err := runtimev2.CanonicalList([]runtimev2.CanonicalValue{inspectString(t, "immutable")})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 20 {
				children, ok := value.Elements()
				if !ok || len(children) != 1 {
					t.Error("invalid concurrent view")
					return
				}
				children[0] = runtimev2.CanonicalNull()
			}
		}()
	}
	workers.Wait()
	children, _ := value.Elements()
	if text, _ := children[0].StringValue(); text != "immutable" {
		t.Fatal("shared state changed")
	}
}

// ES27: fixed framed seeds reach the accessor paths before random mutation.
func FuzzCanonicalValueInspect(f *testing.F) {
	text := inspectString(f, "seed")
	list, _ := runtimev2.CanonicalList([]runtimev2.CanonicalValue{text})
	mapping, _ := runtimev2.CanonicalMap([]runtimev2.CanonicalMapEntry{{Key: "key", Value: list}})
	set, _ := runtimev2.CanonicalSet([]runtimev2.CanonicalValue{text})
	for _, value := range []runtimev2.CanonicalValue{runtimev2.CanonicalNull(), text, list, mapping, set} {
		f.Add(value.CanonicalBytes())
	}
	f.Add([]byte{1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > runtimev2.MaxCanonicalValueBytes {
			return
		}
		value, err := runtimev2.ParseCanonicalValueEnvelope(data)
		if err != nil {
			return
		}
		before := value.CanonicalBytes()
		var visit func(runtimev2.CanonicalValue)
		visit = func(node runtimev2.CanonicalValue) {
			if node.EncodedSize() != len(node.CanonicalBytes()) || node.Kind() == "" {
				t.Fatal("invalid inspection metrics")
			}
			text, stringOK := node.StringValue()
			if stringOK != (node.Kind() == runtimev2.CanonicalValueKindString) || (!stringOK && text != "") {
				t.Fatal("wrong string kind")
			}
			if children, ok := node.Elements(); ok {
				for _, child := range children {
					visit(child)
				}
			}
			if entries, ok := node.Entries(); ok {
				for _, entry := range entries {
					visit(entry.Value)
				}
			}
		}
		visit(value)
		if !bytes.Equal(before, value.CanonicalBytes()) {
			t.Fatal("inspection changed input")
		}
	})
}

func inspectString(t testing.TB, text string) runtimev2.CanonicalValue {
	t.Helper()
	value, err := runtimev2.CanonicalString(text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func inspectTexts(t testing.TB, values []runtimev2.CanonicalValue) []string {
	t.Helper()
	result := make([]string, len(values))
	for index, value := range values {
		text, ok := value.StringValue()
		if !ok {
			t.Fatal("expected string child")
		}
		result[index] = text
	}
	return result
}
