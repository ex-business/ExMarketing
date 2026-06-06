package marketing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type WorkflowValidationService struct {
	nodeConfigSchemas WorkflowNodeConfigSchemaRegistry
}

func NewWorkflowValidationService() WorkflowValidationService {
	return WorkflowValidationService{nodeConfigSchemas: DefaultWorkflowNodeConfigSchemas()}
}

func ValidateWorkflowDefinition(raw json.RawMessage) WorkflowValidationResult {
	return NewWorkflowValidationService().ValidateDefinition(raw)
}

func (s WorkflowValidationService) ValidateDefinition(raw json.RawMessage) WorkflowValidationResult {
	result := WorkflowValidationResult{Valid: true}
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		result.addIssue(WorkflowValidationBlock, "WORKFLOW_DEFINITION_REQUIRED", "workflow definition is required", "definition", "", "")
		return result.finalize()
	}
	var definition WorkflowDefinition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		result.addIssue(WorkflowValidationBlock, "WORKFLOW_DEFINITION_INVALID", err.Error(), "definition", "", "")
		return result.finalize()
	}
	if len(definition.Nodes) == 0 {
		result.addIssue(WorkflowValidationBlock, "WORKFLOW_NODES_REQUIRED", "workflow nodes are required", "nodes", "", "")
	}
	s.validateMetadata(&result, definition.Meta)
	seenNodeIDs := map[string]int{}
	nodeByID := map[string]WorkflowNode{}
	nodeIDs := []string{}
	entryNodeIDs := []string{}
	entryNodeIDSet := map[string]bool{}
	entryCount := 0
	canCheckReachability := true
	for index, node := range definition.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		nodeType := s.nodeType(node)
		if nodeID == "" {
			result.addIssue(WorkflowValidationBlock, "WORKFLOW_NODE_ID_REQUIRED", fmt.Sprintf("nodes[%d].id is required", index), fmt.Sprintf("nodes[%d].id", index), "", "")
			canCheckReachability = false
		} else if _, ok := seenNodeIDs[nodeID]; ok {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_ID_DUPLICATED",
				fmt.Sprintf("节点 ID %q 重复，请保证每个节点 ID 唯一。", nodeID),
				fmt.Sprintf("nodes[%d].id", index),
				nodeID,
				"",
			)
			canCheckReachability = false
		} else {
			seenNodeIDs[nodeID] = index
			nodeByID[nodeID] = node
			nodeIDs = append(nodeIDs, nodeID)
		}
		if nodeType == "" {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_TYPE_REQUIRED",
				fmt.Sprintf("节点 %q 缺少 type。", nodeID),
				fmt.Sprintf("nodes[%d].type", index),
				nodeID,
				"",
			)
		} else if schema, ok := s.nodeConfigSchemas.Get(nodeType); ok {
			s.validateNodeConfig(&result, index, nodeID, node.Config, schema)
		} else {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_TYPE_UNSUPPORTED",
				fmt.Sprintf("节点类型 %q 暂不支持，请更换为已注册的节点类型。", nodeType),
				fmt.Sprintf("nodes[%d].type", index),
				nodeID,
				"",
			)
		}
		if nodeType == "event-in" || strings.TrimSpace(node.Kind) == "entry" {
			entryCount++
			if nodeID != "" {
				entryNodeIDs = append(entryNodeIDs, nodeID)
				entryNodeIDSet[nodeID] = true
			}
		}
	}
	if entryCount == 0 {
		result.addIssue(WorkflowValidationBlock, "WORKFLOW_ENTRY_MISSING", "入口节点不存在，请添加 Event In 节点。", "nodes", "", "")
	}
	adjacency := map[string][]string{}
	hasIncomingEdge := map[string]bool{}
	hasOutgoingEdge := map[string]bool{}
	outgoingEdgeByOutput := map[string]string{}
	outgoingRoutesByNode := map[string]map[string]string{}
	defaultRouteByNode := map[string]string{}
	for index, edge := range definition.Edges {
		edgeID := strings.TrimSpace(edge.ID)
		source := strings.TrimSpace(edge.Source)
		target := strings.TrimSpace(edge.Target)
		routeKey, routeLabel := workflowEdgeRouteKey(edge)
		if edge.Route != nil {
			routeType := edge.Route.NormalizedType()
			if routeType == "" {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_EDGE_ROUTE_TYPE_REQUIRED",
					fmt.Sprintf("连线 %q 的 route.type 不能为空。", edgeID),
					fmt.Sprintf("edges[%d].route.type", index),
					source,
					edgeID,
				)
			} else if !edge.Route.IsSupported() {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_EDGE_ROUTE_TYPE_UNSUPPORTED",
					fmt.Sprintf("连线 %q 的 route.type 不受支持。", edgeID),
					fmt.Sprintf("edges[%d].route.type", index),
					source,
					edgeID,
				)
			}
			if routeType == WorkflowEdgeRouteOutput && edge.Route.OutputKey() == "" {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_EDGE_ROUTE_KEY_REQUIRED",
					fmt.Sprintf("连线 %q 的 OUTPUT route 必须配置 key 或 label。", edgeID),
					fmt.Sprintf("edges[%d].route.key", index),
					source,
					edgeID,
				)
			}
			if workflowEdgeRouteRequiresCondition(edge.Route) {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_EDGE_ROUTE_CONDITION_REQUIRED",
					fmt.Sprintf("连线 %q 的条件路由必须配置 expression 或 conditionRef。", edgeID),
					fmt.Sprintf("edges[%d].route", index),
					source,
					edgeID,
				)
			}
		}
		if source == "" || target == "" {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_EDGE_ENDPOINT_REQUIRED",
				"连线必须同时包含 source 和 target。",
				fmt.Sprintf("edges[%d]", index),
				"",
				edgeID,
			)
			canCheckReachability = false
			continue
		}
		if _, ok := seenNodeIDs[source]; !ok {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_EDGE_SOURCE_MISSING",
				fmt.Sprintf("连线 source 节点 %q 不存在。", source),
				fmt.Sprintf("edges[%d].source", index),
				"",
				edgeID,
			)
			canCheckReachability = false
		}
		if _, ok := seenNodeIDs[target]; !ok {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_EDGE_TARGET_MISSING",
				fmt.Sprintf("连线 target 节点 %q 不存在。", target),
				fmt.Sprintf("edges[%d].target", index),
				"",
				edgeID,
			)
			canCheckReachability = false
		}
		if _, sourceOK := seenNodeIDs[source]; sourceOK {
			if _, targetOK := seenNodeIDs[target]; targetOK {
				adjacency[source] = append(adjacency[source], target)
				hasOutgoingEdge[source] = true
				hasIncomingEdge[target] = true
				routeOutputKey := workflowEdgeRouteOutputKey(edge, nodeByID[source])
				if routeOutputKey != "" {
					if outgoingRoutesByNode[source] == nil {
						outgoingRoutesByNode[source] = map[string]string{}
					}
					outgoingRoutesByNode[source][routeOutputKey] = edgeID
				}
				if workflowEdgeRouteIsDefault(edge) {
					if previousEdgeID, ok := defaultRouteByNode[source]; ok {
						result.addIssue(
							WorkflowValidationBlock,
							"WORKFLOW_BRANCH_DEFAULT_DUPLICATED",
							fmt.Sprintf("分支节点 %q 已存在默认出口 %q，不能配置多个默认分支。", source, previousEdgeID),
							fmt.Sprintf("edges[%d].route.default", index),
							source,
							edgeID,
						)
					} else {
						defaultRouteByNode[source] = edgeID
					}
				}
				if routeKey != "" {
					outputKey := source + "\x00" + routeKey
					if previousEdgeID, ok := outgoingEdgeByOutput[outputKey]; ok {
						result.addIssue(
							WorkflowValidationBlock,
							"WORKFLOW_OUTPUT_DUPLICATED_EDGE",
							fmt.Sprintf("节点 %q 的同一个输出路由 %q 已存在连线 %q，不能重复连出。", source, routeLabel, previousEdgeID),
							fmt.Sprintf("edges[%d].route", index),
							source,
							edgeID,
						)
					} else {
						outgoingEdgeByOutput[outputKey] = edgeID
					}
				}
			}
		}
	}
	upstreamSourceTarget := s.upstreamSourceTargets(nodeIDs, adjacency)
	s.validateNodeInputMappings(&result, definition.Nodes, nodeByID, upstreamSourceTarget)
	islandNodeIDs := map[string]bool{}
	if canCheckReachability && len(entryNodeIDs) > 0 {
		for _, nodeID := range nodeIDs {
			if entryNodeIDSet[nodeID] || hasIncomingEdge[nodeID] || hasOutgoingEdge[nodeID] {
				continue
			}
			islandNodeIDs[nodeID] = true
			nodeIndex := seenNodeIDs[nodeID]
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_ISLAND",
				fmt.Sprintf("节点 %q 是孤岛节点，没有任何输入或输出连线。", nodeID),
				fmt.Sprintf("nodes[%d]", nodeIndex),
				nodeID,
				"",
			)
		}
	}
	if canCheckReachability && len(entryNodeIDs) > 0 {
		reachableNodeIDs := s.reachableNodes(entryNodeIDs, adjacency)
		for _, nodeID := range nodeIDs {
			if reachableNodeIDs[nodeID] || islandNodeIDs[nodeID] {
				continue
			}
			nodeIndex := seenNodeIDs[nodeID]
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_UNREACHABLE",
				fmt.Sprintf("节点 %q 无法从入口节点到达，请检查连线。", nodeID),
				fmt.Sprintf("nodes[%d]", nodeIndex),
				nodeID,
				"",
			)
		}
	}
	if canCheckReachability {
		if cycleNodeID := s.cycleNode(nodeIDs, adjacency); cycleNodeID != "" {
			nodeIndex := seenNodeIDs[cycleNodeID]
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_UNSUPPORTED_CYCLE",
				fmt.Sprintf("检测到不支持的环路，节点 %q 已进入循环链路。", cycleNodeID),
				fmt.Sprintf("nodes[%d]", nodeIndex),
				cycleNodeID,
				"",
			)
		}
	}
	if len(result.Items) == 0 {
		s.validateBranchRouteCoverage(&result, definition.Nodes, outgoingRoutesByNode, defaultRouteByNode)
	}
	return result.finalize()
}

func (s WorkflowValidationService) validateNodeInputMappings(result *WorkflowValidationResult, nodes []WorkflowNode, nodeByID map[string]WorkflowNode, upstreamSourceTarget map[string]bool) {
	for nodeIndex, node := range nodes {
		nodeID := strings.TrimSpace(node.ID)
		if nodeID == "" {
			continue
		}
		schema, ok := s.nodeConfigSchemas.Get(s.nodeType(node))
		if !ok {
			continue
		}
		config, ok := s.nodeConfigMap(node.Config)
		if !ok {
			continue
		}
		rawMapping, hasMapping := config["inputMapping"]
		mapping, mappingOK := rawMapping.(map[string]any)
		if hasMapping && !mappingOK {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_INPUT_MAPPING_INVALID",
				fmt.Sprintf("节点 %q 的 inputMapping 必须是 object。", nodeID),
				fmt.Sprintf("nodes[%d].config.inputMapping", nodeIndex),
				nodeID,
				"",
			)
			continue
		}
		inputsByKey := map[string]WorkflowNodeInputVar{}
		for _, input := range schema.Inputs {
			inputsByKey[input.Key] = input
		}
		if mapping == nil {
			continue
		}
		for inputKey, rawRef := range mapping {
			inputPath := fmt.Sprintf("nodes[%d].config.inputMapping.%s", nodeIndex, inputKey)
			if _, ok := inputsByKey[inputKey]; !ok {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_NODE_INPUT_MAPPING_INPUT_UNSUPPORTED",
					fmt.Sprintf("节点 %q 不支持输入变量 %q。", nodeID, inputKey),
					inputPath,
					nodeID,
					"",
				)
				continue
			}
			ref := mappingRef(rawRef)
			if ref.SourceNodeID == "" || ref.OutputKey == "" {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_NODE_INPUT_MAPPING_INVALID",
					fmt.Sprintf("节点 %q 的输入变量 %q 引用格式无效。", nodeID, inputKey),
					inputPath,
					nodeID,
					"",
				)
				continue
			}
			sourceNode, ok := nodeByID[ref.SourceNodeID]
			if !ok {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_NODE_INPUT_MAPPING_SOURCE_MISSING",
					fmt.Sprintf("节点 %q 的输入变量 %q 引用了不存在的源节点 %q。", nodeID, inputKey, ref.SourceNodeID),
					inputPath+".sourceNodeId",
					nodeID,
					"",
				)
				continue
			}
			if !upstreamSourceTarget[ref.SourceNodeID+"\x00"+nodeID] {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_NODE_INPUT_MAPPING_SOURCE_NOT_CONNECTED",
					fmt.Sprintf("节点 %q 的输入变量 %q 引用了不在上游路径上的源节点 %q。", nodeID, inputKey, ref.SourceNodeID),
					inputPath+".sourceNodeId",
					nodeID,
					"",
				)
			}
			if !s.nodeDeclaresOutput(sourceNode, ref.OutputKey) {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_NODE_INPUT_MAPPING_OUTPUT_MISSING",
					fmt.Sprintf("节点 %q 的输入变量 %q 引用了源节点 %q 未声明的输出 %q。", nodeID, inputKey, ref.SourceNodeID, ref.OutputKey),
					inputPath+".outputKey",
					nodeID,
					"",
				)
			}
			if ref.Path != "" && !strings.HasPrefix(ref.Path, "$") {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_NODE_INPUT_MAPPING_PATH_INVALID",
					fmt.Sprintf("节点 %q 的输入变量 %q 引用 path 必须使用 JSONPath 格式，例如 $.user.id。", nodeID, inputKey),
					inputPath+".path",
					nodeID,
					"",
				)
			}
		}
	}
}

func (s WorkflowValidationService) upstreamSourceTargets(nodeIDs []string, adjacency map[string][]string) map[string]bool {
	out := map[string]bool{}
	for _, source := range nodeIDs {
		seen := map[string]bool{}
		queue := append([]string(nil), adjacency[source]...)
		for len(queue) > 0 {
			target := queue[0]
			queue = queue[1:]
			if target == "" || seen[target] {
				continue
			}
			seen[target] = true
			out[source+"\x00"+target] = true
			queue = append(queue, adjacency[target]...)
		}
	}
	return out
}

func (s WorkflowValidationService) nodeConfigMap(raw json.RawMessage) (map[string]any, bool) {
	config := map[string]any{}
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return config, true
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, false
	}
	return config, true
}

func mappingRef(value any) WorkflowNodeInputMappingRef {
	item, ok := value.(map[string]any)
	if !ok {
		return WorkflowNodeInputMappingRef{}
	}
	sourceNodeID, _ := item["sourceNodeId"].(string)
	outputKey, _ := item["outputKey"].(string)
	path, _ := item["path"].(string)
	if sourceNodeID == "" {
		sourceNodeID, _ = item["nodeId"].(string)
	}
	if outputKey == "" {
		outputKey, _ = item["sourceOutputKey"].(string)
	}
	if outputKey == "" {
		outputKey, _ = item["output"].(string)
	}
	return WorkflowNodeInputMappingRef{SourceNodeID: strings.TrimSpace(sourceNodeID), OutputKey: strings.TrimSpace(outputKey), Path: strings.TrimSpace(path)}
}

func workflowEdgeRouteKey(edge WorkflowEdge) (string, string) {
	if edge.Route == nil {
		fallback := strings.TrimSpace(edge.SourceHandle)
		if fallback == "" {
			fallback = strings.TrimSpace(edge.Label)
		}
		if fallback == "" {
			fallback = "DEFAULT"
		}
		return "LEGACY:" + strings.ToUpper(fallback), fallback
	}
	routeType := string(edge.Route.NormalizedType())
	label := strings.TrimSpace(edge.Route.Label)
	if routeType == "" || !edge.Route.IsSupported() {
		return "", label
	}
	if edge.Route.NormalizedType() == WorkflowEdgeRouteOutput && edge.Route.OutputKey() == "" {
		return "", label
	}
	key := strings.TrimSpace(edge.Route.Key)
	if key == "" {
		key = label
	}
	if key == "" {
		key = routeType
	}
	if label == "" {
		label = key
	}
	return routeType + ":" + strings.ToUpper(key), label
}

func workflowEdgeRouteRequiresCondition(route *WorkflowEdgeRoute) bool {
	if route == nil {
		return false
	}
	return route.RequiresCondition()
}

func workflowEdgeRouteIsDefault(edge WorkflowEdge) bool {
	if edge.Route == nil {
		return false
	}
	return edge.Route.IsDefault()
}

func workflowEdgeRouteOutputKey(edge WorkflowEdge, sourceNode WorkflowNode) string {
	if edge.Route != nil {
		if value := edge.Route.OutputKey(); value != "" {
			return value
		}
	}
	for _, value := range []string{edge.Label, edge.SourceHandle} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "output-") {
			if outputKey := workflowNodeOutputByHandle(sourceNode, value); outputKey != "" {
				return outputKey
			}
		}
		return strings.ToUpper(value)
	}
	return ""
}

func workflowNodeOutputByHandle(node WorkflowNode, sourceHandle string) string {
	indexText := strings.TrimPrefix(sourceHandle, "output-")
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 {
		return ""
	}
	outputs := node.Outputs
	if len(outputs) == 0 {
		schema, ok := DefaultWorkflowNodeConfigSchemas().Get(strings.TrimSpace(node.Type))
		if ok {
			for _, output := range schema.Outputs {
				outputs = append(outputs, output.Key)
			}
		}
	}
	if index >= len(outputs) {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(outputs[index]))
}

func (s WorkflowValidationService) validateBranchRouteCoverage(result *WorkflowValidationResult, nodes []WorkflowNode, outgoingRoutesByNode map[string]map[string]string, defaultRouteByNode map[string]string) {
	for index, node := range nodes {
		nodeID := strings.TrimSpace(node.ID)
		required := requiredBranchRouteOutputs(s.nodeType(node))
		if nodeID == "" || len(required) == 0 {
			continue
		}
		covered := outgoingRoutesByNode[nodeID]
		for _, outputKey := range required {
			if covered != nil {
				if _, ok := covered[outputKey]; ok {
					continue
				}
			}
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_BRANCH_ROUTE_MISSING",
				fmt.Sprintf("分支节点 %q 缺少出口路由 %q。", nodeID, outputKey),
				fmt.Sprintf("nodes[%d].outputs.%s", index, outputKey),
				nodeID,
				"",
			)
		}
		if _, ok := defaultRouteByNode[nodeID]; !ok {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_BRANCH_DEFAULT_MISSING",
				fmt.Sprintf("分支节点 %q 必须显式配置默认出口。", nodeID),
				fmt.Sprintf("nodes[%d].route.default", index),
				nodeID,
				"",
			)
		}
	}
}

func requiredBranchRouteOutputs(nodeType string) []string {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "condition":
		return []string{"IF", "ELSE"}
	case "qualify":
		return []string{"PASS", "BLOCK", "RISK"}
	case "switch":
		return []string{"PASS", "BLOCK", "SKIP"}
	default:
		return nil
	}
}

func (s WorkflowValidationService) nodeDeclaresOutput(node WorkflowNode, outputKey string) bool {
	schema, ok := s.nodeConfigSchemas.Get(s.nodeType(node))
	if ok {
		for _, output := range schema.Outputs {
			if output.Key == outputKey {
				return true
			}
		}
		return false
	}
	for _, output := range node.Outputs {
		if output == outputKey {
			return true
		}
	}
	return false
}

func (s WorkflowValidationService) nodeType(node WorkflowNode) string {
	for _, value := range []string{node.Type, node.Code, node.Kind} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s WorkflowValidationService) validateNodeConfig(result *WorkflowValidationResult, nodeIndex int, nodeID string, raw json.RawMessage, schema WorkflowNodeConfigSchema) {
	config := map[string]any{}
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
		if err := json.Unmarshal(raw, &config); err != nil {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_CONFIG_INVALID",
				fmt.Sprintf("节点 %q 的 config 必须是 JSON object。", nodeID),
				fmt.Sprintf("nodes[%d].config", nodeIndex),
				nodeID,
				"",
			)
			return
		}
	}
	for _, field := range schema.Fields {
		value, exists := config[field.Key]
		path := fmt.Sprintf("nodes[%d].config.%s", nodeIndex, field.Key)
		if !exists || s.isEmptyConfigValue(value) {
			if defaultValue, hasDefault := s.defaultConfigValue(field); hasDefault {
				if !s.configValueMatchesField(defaultValue, field) {
					result.addIssue(
						WorkflowValidationBlock,
						"WORKFLOW_NODE_CONFIG_DEFAULT_INVALID",
						fmt.Sprintf("节点 %q 的配置 %q 默认值类型不正确，应为 %s。", nodeID, field.Key, field.Type),
						path,
						nodeID,
						"",
					)
					continue
				}
				if field.Type == WorkflowNodeConfigEnum && !s.enumValueAllowed(defaultValue, field.Options) {
					result.addIssue(
						WorkflowValidationBlock,
						"WORKFLOW_NODE_CONFIG_ENUM_INVALID",
						fmt.Sprintf("节点 %q 的配置 %q 默认值不在允许范围内。", nodeID, field.Key),
						path,
						nodeID,
						"",
					)
					continue
				}
				s.validateConfigRules(result, nodeID, path, field, defaultValue)
				continue
			}
			if field.Required {
				result.addIssue(
					WorkflowValidationBlock,
					"WORKFLOW_NODE_CONFIG_REQUIRED",
					fmt.Sprintf("节点 %q 缺少必填配置 %q。", nodeID, field.Key),
					path,
					nodeID,
					"",
				)
			}
			continue
		}
		if !s.configValueMatchesField(value, field) {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_CONFIG_TYPE_INVALID",
				fmt.Sprintf("节点 %q 的配置 %q 类型不正确，应为 %s。", nodeID, field.Key, field.Type),
				path,
				nodeID,
				"",
			)
			continue
		}
		if field.Type == WorkflowNodeConfigEnum && !s.enumValueAllowed(value, field.Options) {
			result.addIssue(
				WorkflowValidationBlock,
				"WORKFLOW_NODE_CONFIG_ENUM_INVALID",
				fmt.Sprintf("节点 %q 的配置 %q 不在允许范围内。", nodeID, field.Key),
				path,
				nodeID,
				"",
			)
		}
		s.validateConfigRules(result, nodeID, path, field, value)
	}
}

func (s WorkflowValidationService) defaultConfigValue(field WorkflowNodeConfigField) (any, bool) {
	if field.Default == nil || s.isEmptyConfigValue(field.Default) {
		return nil, false
	}
	encoded, err := json.Marshal(field.Default)
	if err != nil {
		return field.Default, true
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return field.Default, true
	}
	if s.isEmptyConfigValue(decoded) {
		return nil, false
	}
	return decoded, true
}

func (s WorkflowValidationService) isEmptyConfigValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	default:
		return false
	}
}

func (s WorkflowValidationService) configValueMatchesField(value any, field WorkflowNodeConfigField) bool {
	switch field.Type {
	case WorkflowNodeConfigString, WorkflowNodeConfigEnum:
		_, ok := value.(string)
		return ok
	case WorkflowNodeConfigNumber:
		_, ok := value.(float64)
		return ok
	case WorkflowNodeConfigBoolean:
		_, ok := value.(bool)
		return ok
	case WorkflowNodeConfigArray:
		_, ok := value.([]any)
		return ok
	case WorkflowNodeConfigObject:
		_, ok := value.(map[string]any)
		return ok
	default:
		return false
	}
}

func (s WorkflowValidationService) enumValueAllowed(value any, options []string) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	for _, option := range options {
		if text == option {
			return true
		}
	}
	return false
}

func (s WorkflowValidationService) validateConfigRules(result *WorkflowValidationResult, nodeID string, path string, field WorkflowNodeConfigField, value any) {
	for _, rule := range field.ValidationRules {
		if s.configRulePasses(rule, value) {
			continue
		}
		message := strings.TrimSpace(rule.Message)
		if message == "" {
			message = fmt.Sprintf("节点 %q 的配置 %q 不符合校验规则 %s。", nodeID, field.Key, rule.Type)
		}
		result.addIssue(
			WorkflowValidationBlock,
			"WORKFLOW_NODE_CONFIG_RULE_VIOLATION",
			message,
			path,
			nodeID,
			"",
		)
	}
}

func (s WorkflowValidationService) configRulePasses(rule WorkflowNodeConfigValidationRule, value any) bool {
	switch rule.Type {
	case WorkflowNodeConfigRuleMinLength:
		text, ok := value.(string)
		return ok && len(text) >= intRuleValue(rule.Value)
	case WorkflowNodeConfigRuleMaxLength:
		text, ok := value.(string)
		return ok && len(text) <= intRuleValue(rule.Value)
	case WorkflowNodeConfigRulePattern:
		text, ok := value.(string)
		if !ok {
			return false
		}
		pattern, ok := rule.Value.(string)
		if !ok || strings.TrimSpace(pattern) == "" {
			return false
		}
		matched, err := regexp.MatchString(pattern, text)
		return err == nil && matched
	case WorkflowNodeConfigRuleMinValue:
		number, ok := value.(float64)
		return ok && number >= floatRuleValue(rule.Value)
	case WorkflowNodeConfigRuleMaxValue:
		number, ok := value.(float64)
		return ok && number <= floatRuleValue(rule.Value)
	case WorkflowNodeConfigRuleMinItems:
		items, ok := value.([]any)
		return ok && len(items) >= intRuleValue(rule.Value)
	case WorkflowNodeConfigRuleMaxItems:
		items, ok := value.([]any)
		return ok && len(items) <= intRuleValue(rule.Value)
	default:
		return true
	}
}

func intRuleValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func floatRuleValue(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}

func (s WorkflowValidationService) validateMetadata(result *WorkflowValidationResult, meta map[string]any) {
	requiredFields := []struct {
		key  string
		code string
	}{
		{key: "activityId", code: "WORKFLOW_METADATA_ACTIVITY_ID_REQUIRED"},
		{key: "workspaceId", code: "WORKFLOW_METADATA_WORKSPACE_ID_REQUIRED"},
		{key: "folderId", code: "WORKFLOW_METADATA_FOLDER_ID_REQUIRED"},
	}
	for _, field := range requiredFields {
		if s.metadataString(meta, field.key) != "" {
			continue
		}
		result.addIssue(
			WorkflowValidationBlock,
			field.code,
			fmt.Sprintf("workflow meta.%s is required", field.key),
			"meta."+field.key,
			"",
			"",
		)
	}
}

func (s WorkflowValidationService) metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func (s WorkflowValidationService) reachableNodes(entryNodeIDs []string, adjacency map[string][]string) map[string]bool {
	reachable := map[string]bool{}
	queue := append([]string(nil), entryNodeIDs...)
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if reachable[nodeID] {
			continue
		}
		reachable[nodeID] = true
		queue = append(queue, adjacency[nodeID]...)
	}
	return reachable
}

func (s WorkflowValidationService) cycleNode(nodeIDs []string, adjacency map[string][]string) string {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := map[string]int{}
	var visit func(string) string
	visit = func(nodeID string) string {
		switch state[nodeID] {
		case visiting:
			return nodeID
		case visited:
			return ""
		}
		state[nodeID] = visiting
		for _, nextNodeID := range adjacency[nodeID] {
			if cycleNodeID := visit(nextNodeID); cycleNodeID != "" {
				return cycleNodeID
			}
		}
		state[nodeID] = visited
		return ""
	}
	for _, nodeID := range nodeIDs {
		if cycleNodeID := visit(nodeID); cycleNodeID != "" {
			return cycleNodeID
		}
	}
	return ""
}

func (result *WorkflowValidationResult) addIssue(severity WorkflowValidationSeverity, code string, message string, path string, nodeID string, edgeID string) {
	result.Items = append(result.Items, WorkflowValidationIssue{
		Severity: severity,
		Code:     code,
		Message:  message,
		Path:     path,
		NodeID:   nodeID,
		EdgeID:   edgeID,
	})
}

func (result WorkflowValidationResult) finalize() WorkflowValidationResult {
	for _, item := range result.Items {
		switch item.Severity {
		case WorkflowValidationWarning:
			result.Summary.WarningCount++
		default:
			result.Summary.BlockCount++
		}
	}
	result.Valid = result.Summary.BlockCount == 0
	return result
}
