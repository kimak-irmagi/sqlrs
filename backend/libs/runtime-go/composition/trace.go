package composition

import (
	"encoding/json"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

type traceNodeData struct {
	id               int
	parentID         int
	hasParent        bool
	referencePointer string
	kind             AliasKind
	alias            string
	sourceID         string
	pointer          string
}

type originData struct {
	outputIndex int
	nodeID      int
	pointer     string
}

type traceData struct {
	nodes      []traceNodeData
	factory    *originData
	transforms []originData
}

// ExpansionTrace is an immutable, versioned, non-semantic provenance graph.
type ExpansionTrace struct{ data *traceData }

// TraceNode is one immutable alias occurrence.
type TraceNode struct{ data *traceNodeData }

// FactoryOrigin locates the expanded factory declaration.
type FactoryOrigin struct{ data *originData }

// TransformOrigin locates one expanded transform declaration.
type TransformOrigin struct{ data *originData }

// TargetNodeID returns the canonical target node index. Zero traces also return zero.
func (t ExpansionTrace) TargetNodeID() int { return 0 }

// Nodes returns defensive occurrence values in canonical order.
func (t ExpansionTrace) Nodes() []TraceNode {
	if t.data == nil {
		return nil
	}
	result := make([]TraceNode, len(t.data.nodes))
	for index := range t.data.nodes {
		value := t.data.nodes[index]
		result[index] = TraceNode{data: &value}
	}
	return result
}

// Factory returns the factory origin and whether this is a recipe trace.
func (t ExpansionTrace) Factory() (FactoryOrigin, bool) {
	if t.data == nil || t.data.factory == nil {
		return FactoryOrigin{}, false
	}
	value := *t.data.factory
	return FactoryOrigin{data: &value}, true
}

// Transforms returns defensive origins in output order.
func (t ExpansionTrace) Transforms() []TransformOrigin {
	if t.data == nil {
		return nil
	}
	result := make([]TransformOrigin, len(t.data.transforms))
	for index := range t.data.transforms {
		value := t.data.transforms[index]
		result[index] = TransformOrigin{data: &value}
	}
	return result
}

func (n TraceNode) ID() int {
	if n.data == nil {
		return 0
	}
	return n.data.id
}
func (n TraceNode) ParentID() (int, bool) {
	if n.data == nil || !n.data.hasParent {
		return 0, false
	}
	return n.data.parentID, true
}
func (n TraceNode) ReferencePointer() string {
	if n.data == nil {
		return ""
	}
	return n.data.referencePointer
}
func (n TraceNode) Kind() AliasKind {
	if n.data == nil {
		return ""
	}
	return n.data.kind
}
func (n TraceNode) Alias() string {
	if n.data == nil {
		return ""
	}
	return n.data.alias
}
func (n TraceNode) SourceID() string {
	if n.data == nil {
		return ""
	}
	return n.data.sourceID
}
func (n TraceNode) Pointer() string {
	if n.data == nil {
		return ""
	}
	return n.data.pointer
}
func (o FactoryOrigin) NodeID() int {
	if o.data == nil {
		return 0
	}
	return o.data.nodeID
}
func (o FactoryOrigin) Pointer() string {
	if o.data == nil {
		return ""
	}
	return o.data.pointer
}
func (o TransformOrigin) OutputIndex() int {
	if o.data == nil {
		return 0
	}
	return o.data.outputIndex
}
func (o TransformOrigin) NodeID() int {
	if o.data == nil {
		return 0
	}
	return o.data.nodeID
}
func (o TransformOrigin) Pointer() string {
	if o.data == nil {
		return ""
	}
	return o.data.pointer
}

type traceWire struct {
	SchemaVersion *string                `json:"schema_version"`
	TargetNode    *int                   `json:"target_node"`
	Nodes         *[]traceNodeWire       `json:"nodes"`
	Factory       *factoryOriginWire     `json:"factory,omitempty"`
	Transforms    *[]transformOriginWire `json:"transforms"`
}

type traceNodeWire struct {
	NodeID           *int       `json:"node_id"`
	ParentNodeID     *int       `json:"parent_node_id,omitempty"`
	ReferencePointer *string    `json:"reference_pointer,omitempty"`
	Kind             *AliasKind `json:"kind"`
	Alias            *string    `json:"alias"`
	SourceID         *string    `json:"source_id"`
	Pointer          *string    `json:"pointer"`
}

type factoryOriginWire struct {
	NodeID  *int    `json:"node_id"`
	Pointer *string `json:"pointer"`
}

type transformOriginWire struct {
	OutputIndex *int    `json:"output_index"`
	NodeID      *int    `json:"node_id"`
	Pointer     *string `json:"pointer"`
}

// MarshalJSON emits the strict trace transport.
func (t ExpansionTrace) MarshalJSON() ([]byte, error) {
	if t.data == nil {
		return nil, invalid(CodeInvalidDocument, "", "", nil)
	}
	wire := traceToWire(t.data)
	// The wire graph contains only bounded scalar and slice values.
	raw, _ := json.Marshal(wire)
	if len(raw) > MaxTraceJSONBytes {
		return nil, invalid(CodeExpansionTooLarge, "", "", nil)
	}
	return raw, nil
}

// UnmarshalJSON strictly decodes and atomically validates a trace.
func (t *ExpansionTrace) UnmarshalJSON(raw []byte) error {
	return DecodeExpansionTraceJSON(raw, t)
}

// DecodeExpansionTraceJSON strictly decodes one complete bounded trace.
func DecodeExpansionTraceJSON(raw []byte, target *ExpansionTrace) error {
	if target == nil {
		return invalid(CodeInvalidDocument, "", "", nil)
	}
	var wire traceWire
	if err := decodeStrictJSON(raw, &wire, MaxTraceJSONBytes, "", ""); err != nil {
		return err
	}
	value, err := traceFromWire(wire)
	if err != nil {
		return err
	}
	*target = ExpansionTrace{data: value}
	return nil
}

func traceToWire(data *traceData) traceWire {
	version, target := ExpansionTraceSchemaVersion, 0
	nodes := make([]traceNodeWire, len(data.nodes))
	for index, node := range data.nodes {
		id, kind, alias, sourceID, pointer := node.id, node.kind, node.alias, node.sourceID, node.pointer
		nodes[index] = traceNodeWire{NodeID: &id, Kind: &kind, Alias: &alias, SourceID: &sourceID, Pointer: &pointer}
		if node.hasParent {
			parent, reference := node.parentID, node.referencePointer
			nodes[index].ParentNodeID = &parent
			nodes[index].ReferencePointer = &reference
		}
	}
	transforms := make([]transformOriginWire, len(data.transforms))
	for index, origin := range data.transforms {
		outputIndex, nodeID, pointer := origin.outputIndex, origin.nodeID, origin.pointer
		transforms[index] = transformOriginWire{OutputIndex: &outputIndex, NodeID: &nodeID, Pointer: &pointer}
	}
	wire := traceWire{SchemaVersion: &version, TargetNode: &target, Nodes: &nodes, Transforms: &transforms}
	if data.factory != nil {
		nodeID, pointer := data.factory.nodeID, data.factory.pointer
		wire.Factory = &factoryOriginWire{NodeID: &nodeID, Pointer: &pointer}
	}
	return wire
}

func traceFromWire(wire traceWire) (*traceData, error) {
	if wire.SchemaVersion == nil || *wire.SchemaVersion != ExpansionTraceSchemaVersion || wire.TargetNode == nil || *wire.TargetNode != 0 || wire.Nodes == nil || wire.Transforms == nil {
		return nil, invalid(CodeInvalidDocument, "", "", nil)
	}
	if len(*wire.Nodes) > MaxTraceNodes || len(*wire.Transforms) > runtimev2.MaxTransforms {
		return nil, invalid(CodeExpansionTooLarge, "", "", nil)
	}
	data := &traceData{nodes: make([]traceNodeData, len(*wire.Nodes)), transforms: make([]originData, len(*wire.Transforms))}
	for index, node := range *wire.Nodes {
		if node.NodeID == nil || node.Kind == nil || node.Alias == nil || node.SourceID == nil || node.Pointer == nil || *node.NodeID != index || !validAliasName(*node.Alias) || !validateSourceID(*node.SourceID) || !validatePointer(*node.Pointer) {
			return nil, invalid(CodeInvalidDocument, "", "", nil)
		}
		data.nodes[index] = traceNodeData{id: index, kind: *node.Kind, alias: *node.Alias, sourceID: *node.SourceID, pointer: *node.Pointer}
		if node.ParentNodeID != nil || node.ReferencePointer != nil {
			if node.ParentNodeID == nil || node.ReferencePointer == nil || !validatePointer(*node.ReferencePointer) {
				return nil, invalid(CodeInvalidDocument, "", "", nil)
			}
			data.nodes[index].hasParent = true
			data.nodes[index].parentID = *node.ParentNodeID
			data.nodes[index].referencePointer = *node.ReferencePointer
		}
	}
	if wire.Factory != nil {
		if wire.Factory.NodeID == nil || wire.Factory.Pointer == nil || !validatePointer(*wire.Factory.Pointer) {
			return nil, invalid(CodeInvalidDocument, "", "", nil)
		}
		data.factory = &originData{nodeID: *wire.Factory.NodeID, pointer: *wire.Factory.Pointer}
	}
	for index, origin := range *wire.Transforms {
		if origin.OutputIndex == nil || origin.NodeID == nil || origin.Pointer == nil || *origin.OutputIndex != index || !validatePointer(*origin.Pointer) {
			return nil, invalid(CodeInvalidDocument, "", "", nil)
		}
		data.transforms[index] = originData{outputIndex: index, nodeID: *origin.NodeID, pointer: *origin.Pointer}
	}
	if err := validateTraceData(data); err != nil {
		return nil, err
	}
	return data, nil
}

func validateTraceData(data *traceData) error {
	if len(data.nodes) == 0 {
		return invalid(CodeInvalidDocument, "", "", nil)
	}
	for index, node := range data.nodes {
		if node.pointer != definitionPointer(node.alias) {
			return invalid(CodeInvalidDocument, node.sourceID, node.pointer, nil)
		}
		if index == 0 {
			if node.hasParent {
				return invalid(CodeInvalidDocument, node.sourceID, node.pointer, nil)
			}
		} else if !node.hasParent || node.parentID < 0 || node.parentID >= index {
			return invalid(CodeInvalidDocument, node.sourceID, node.pointer, nil)
		}
	}
	if data.nodes[0].kind == AliasKindTransform {
		return validateStandaloneTrace(data)
	}
	if data.nodes[0].kind != AliasKindRecipe {
		return invalid(CodeInvalidDocument, data.nodes[0].sourceID, data.nodes[0].pointer, nil)
	}
	return validateRecipeTrace(data)
}

func validateStandaloneTrace(data *traceData) error {
	if len(data.nodes) != 1 || data.factory != nil || len(data.transforms) != 1 {
		return invalid(CodeInvalidDocument, "", "", nil)
	}
	origin := data.transforms[0]
	if origin.nodeID != 0 || origin.pointer != data.nodes[0].pointer+"/declaration" {
		return invalid(CodeInvalidDocument, data.nodes[0].sourceID, origin.pointer, nil)
	}
	return nil
}

func validateRecipeTrace(data *traceData) error {
	recipeCount := 0
	for recipeCount < len(data.nodes) && data.nodes[recipeCount].kind == AliasKindRecipe {
		node := data.nodes[recipeCount]
		if recipeCount > 0 {
			parent := data.nodes[recipeCount-1]
			if node.parentID != recipeCount-1 || node.referencePointer != parent.pointer+"/base/recipe" {
				return invalid(CodeInvalidDocument, node.sourceID, node.referencePointer, nil)
			}
		}
		recipeCount++
	}
	if recipeCount == 0 || data.factory == nil || data.factory.nodeID != recipeCount-1 || data.factory.pointer != data.nodes[recipeCount-1].pointer+"/base/factory" {
		return invalid(CodeInvalidDocument, "", "", nil)
	}
	for index := recipeCount; index < len(data.nodes); index++ {
		node := data.nodes[index]
		if node.kind != AliasKindTransform || node.parentID < 0 || node.parentID >= recipeCount {
			return invalid(CodeInvalidDocument, node.sourceID, node.pointer, nil)
		}
	}

	used := make([]bool, len(data.nodes))
	nextTransformNode := recipeCount
	lastRecipe, lastStep := recipeCount, -1
	for _, origin := range data.transforms {
		if origin.nodeID < 0 || origin.nodeID >= len(data.nodes) {
			return invalid(CodeInvalidDocument, "", origin.pointer, nil)
		}
		node := data.nodes[origin.nodeID]
		recipeID, stepIndex := origin.nodeID, 0
		if node.kind == AliasKindTransform {
			if origin.nodeID != nextTransformNode || used[origin.nodeID] || origin.pointer != node.pointer+"/declaration" {
				return invalid(CodeInvalidDocument, node.sourceID, origin.pointer, nil)
			}
			recipeID = node.parentID
			var ok bool
			stepIndex, ok = parseStepPointer(node.referencePointer, data.nodes[recipeID].alias, "use")
			if !ok {
				return invalid(CodeInvalidDocument, node.sourceID, node.referencePointer, nil)
			}
			used[origin.nodeID] = true
			nextTransformNode++
		} else if node.kind == AliasKindRecipe {
			var ok bool
			stepIndex, ok = parseStepPointer(origin.pointer, node.alias, "transform")
			if !ok {
				return invalid(CodeInvalidDocument, node.sourceID, origin.pointer, nil)
			}
		} else {
			return invalid(CodeInvalidDocument, node.sourceID, origin.pointer, nil)
		}
		if recipeID > lastRecipe {
			return invalid(CodeInvalidDocument, node.sourceID, origin.pointer, nil)
		}
		if recipeID < lastRecipe {
			lastRecipe, lastStep = recipeID, -1
		}
		if stepIndex != lastStep+1 {
			return invalid(CodeInvalidDocument, node.sourceID, origin.pointer, nil)
		}
		lastStep = stepIndex
	}
	if nextTransformNode != len(data.nodes) {
		return invalid(CodeInvalidDocument, "", "", nil)
	}
	return nil
}

func newTrace(data traceData) (ExpansionTrace, error) {
	if len(data.nodes) > MaxTraceNodes || len(data.transforms) > runtimev2.MaxTransforms {
		return ExpansionTrace{}, invalid(CodeExpansionTooLarge, "", "", nil)
	}
	if err := validateTraceData(&data); err != nil {
		return ExpansionTrace{}, err
	}
	trace := ExpansionTrace{data: cloneTraceData(&data)}
	raw, _ := json.Marshal(traceToWire(trace.data))
	if len(raw) > MaxTraceJSONBytes {
		return ExpansionTrace{}, invalid(CodeExpansionTooLarge, "", "", nil)
	}
	return trace, nil
}

func cloneTraceData(source *traceData) *traceData {
	result := &traceData{nodes: append([]traceNodeData(nil), source.nodes...), transforms: append([]originData(nil), source.transforms...)}
	if source.factory != nil {
		factory := *source.factory
		result.factory = &factory
	}
	return result
}
