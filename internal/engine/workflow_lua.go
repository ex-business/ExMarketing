package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	lua "github.com/yuin/gopher-lua"
)

const (
	defaultLuaOutputMaxBytes = 64 * 1024
	hardLuaOutputMaxBytes    = 1024 * 1024
)

func executeWorkflowLua(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, source string) (map[string]any, error) {
	executionContext, _ := ctx.Value(workflowNodeExecutionContextKey{}).(workflowNodeExecutionContext)
	state := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer state.Close()
	state.SetContext(ctx)
	lua.OpenBase(state)
	lua.OpenTable(state)
	lua.OpenString(state)
	lua.OpenMath(state)
	for _, name := range []string{"dofile", "loadfile", "load", "require", "collectgarbage", "print"} {
		state.SetGlobal(name, lua.LNil)
	}

	var emitted map[string]any
	nodeVars := map[string]any{}
	state.SetGlobal("emit", state.NewFunction(func(l *lua.LState) int {
		value := l.CheckAny(1)
		object, ok := luaValueToGo(value).(map[string]any)
		if !ok {
			l.RaiseError("emit expects a table object")
			return 0
		}
		emitted = object
		return 0
	}))
	state.SetGlobal("set_node_var", state.NewFunction(func(l *lua.LState) int {
		key := l.CheckString(1)
		nodeVars[key] = luaValueToGo(l.CheckAny(2))
		return 0
	}))
	state.SetGlobal("get_node_var", state.NewFunction(func(l *lua.LState) int {
		key := l.CheckString(1)
		l.Push(goValueToLua(state, nodeVars[key]))
		return 1
	}))
	state.SetGlobal("set_global", state.NewFunction(func(l *lua.LState) int {
		key := l.CheckString(1)
		if executionContext.Variables == nil {
			l.RaiseError("workflow variable table is not available")
			return 0
		}
		workflowSetVar(executionContext.Variables, "execution", key, luaValueToGo(l.CheckAny(2)))
		return 0
	}))
	state.SetGlobal("get_global", state.NewFunction(func(l *lua.LState) int {
		key := l.CheckString(1)
		l.Push(goValueToLua(state, workflowGetVar(executionContext.Variables, "execution", key)))
		return 1
	}))
	state.SetGlobal("set_var", state.NewFunction(func(l *lua.LState) int {
		scope := strings.ToLower(strings.TrimSpace(l.CheckString(1)))
		key := l.CheckString(2)
		value := luaValueToGo(l.CheckAny(3))
		switch scope {
		case "node", "local":
			nodeVars[key] = value
		case "global", "workflow":
			if executionContext.Variables == nil {
				l.RaiseError("workflow variable table is not available")
				return 0
			}
			workflowSetVar(executionContext.Variables, "execution", key, value)
		case "execution", "activity", "user", "tenant":
			if executionContext.Variables == nil {
				l.RaiseError("workflow variable table is not available")
				return 0
			}
			workflowSetVar(executionContext.Variables, scope, key, value)
		default:
			l.RaiseError("unsupported variable scope: %s", scope)
		}
		return 0
	}))
	state.SetGlobal("get_var", state.NewFunction(func(l *lua.LState) int {
		scope := strings.ToLower(strings.TrimSpace(l.CheckString(1)))
		key := l.CheckString(2)
		switch scope {
		case "node", "local":
			l.Push(goValueToLua(state, nodeVars[key]))
		case "global", "workflow":
			l.Push(goValueToLua(state, workflowGetVar(executionContext.Variables, "execution", key)))
		case "execution", "activity", "user", "tenant":
			l.Push(goValueToLua(state, workflowGetVar(executionContext.Variables, scope, key)))
		default:
			l.Push(lua.LNil)
		}
		return 1
	}))
	state.SetGlobal("node_output", state.NewFunction(func(l *lua.LState) int {
		nodeID := l.CheckString(1)
		key := l.CheckString(2)
		if output, ok := executionContext.NodeOutputs[nodeID]; ok {
			l.Push(goValueToLua(state, output[key]))
			return 1
		}
		l.Push(lua.LNil)
		return 1
	}))
	state.SetGlobal("input", state.NewFunction(func(l *lua.LState) int {
		key := l.CheckString(1)
		l.Push(goValueToLua(state, executionContext.ResolvedInput[key]))
		return 1
	}))
	state.SetGlobal("event_attr", state.NewFunction(func(l *lua.LState) int {
		key := l.CheckString(1)
		l.Push(goValueToLua(state, runtime.Inbox.Event.Attributes[key]))
		return 1
	}))
	state.SetGlobal("get_path", state.NewFunction(func(l *lua.LState) int {
		root := l.CheckAny(1)
		path := l.CheckString(2)
		l.Push(luaGetPath(root, path))
		return 1
	}))
	state.SetGlobal("coalesce", state.NewFunction(func(l *lua.LState) int {
		for i := 1; i <= l.GetTop(); i++ {
			value := l.Get(i)
			if !luaValueIsEmpty(value) {
				l.Push(value)
				return 1
			}
		}
		l.Push(lua.LNil)
		return 1
	}))
	state.SetGlobal("is_empty", state.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LBool(luaValueIsEmpty(l.CheckAny(1))))
		return 1
	}))
	state.SetGlobal("to_number", state.NewFunction(func(l *lua.LState) int {
		value := l.CheckAny(1)
		switch typed := value.(type) {
		case lua.LNumber:
			l.Push(typed)
		case lua.LString:
			number, err := strconv.ParseFloat(strings.TrimSpace(string(typed)), 64)
			if err != nil {
				l.Push(lua.LNil)
			} else {
				l.Push(lua.LNumber(number))
			}
		default:
			l.Push(lua.LNil)
		}
		return 1
	}))
	state.SetGlobal("to_string", state.NewFunction(func(l *lua.LState) int {
		value := l.CheckAny(1)
		if value == lua.LNil {
			l.Push(lua.LString(""))
		} else {
			l.Push(lua.LString(value.String()))
		}
		return 1
	}))
	registerLuaDecimalFunctions(state)
	registerLuaTimeFunctions(state)
	state.SetGlobal("ctx", goValueToLua(state, workflowLuaContext(ctx, node, runtime)))

	if err := state.DoString(source); err != nil {
		return nil, err
	}
	if emitted == nil {
		emitted = map[string]any{}
	}
	for key, value := range nodeVars {
		emitted[key] = value
	}
	if len(nodeVars) > 0 {
		emitted["vars"] = cloneWorkflowVars(nodeVars)
	}
	if err := enforceLuaOutputLimit(node, emitted); err != nil {
		return nil, err
	}
	return emitted, nil
}

func registerLuaDecimalFunctions(state *lua.LState) {
	state.SetGlobal("decimal_add", state.NewFunction(func(l *lua.LState) int {
		return luaDecimalBinary(l, func(a decimal.Decimal, b decimal.Decimal) (decimal.Decimal, error) { return a.Add(b), nil })
	}))
	state.SetGlobal("decimal_sub", state.NewFunction(func(l *lua.LState) int {
		return luaDecimalBinary(l, func(a decimal.Decimal, b decimal.Decimal) (decimal.Decimal, error) { return a.Sub(b), nil })
	}))
	state.SetGlobal("decimal_mul", state.NewFunction(func(l *lua.LState) int {
		return luaDecimalBinary(l, func(a decimal.Decimal, b decimal.Decimal) (decimal.Decimal, error) { return a.Mul(b), nil })
	}))
	state.SetGlobal("decimal_div", state.NewFunction(func(l *lua.LState) int {
		return luaDecimalBinary(l, func(a decimal.Decimal, b decimal.Decimal) (decimal.Decimal, error) { return a.Div(b) })
	}))
	state.SetGlobal("decimal_compare", state.NewFunction(func(l *lua.LState) int {
		left, err := luaDecimalArg(l, 1)
		if err != nil {
			l.RaiseError("%s", err.Error())
			return 0
		}
		right, err := luaDecimalArg(l, 2)
		if err != nil {
			l.RaiseError("%s", err.Error())
			return 0
		}
		l.Push(lua.LNumber(left.Cmp(right)))
		return 1
	}))
}

func registerLuaTimeFunctions(state *lua.LState) {
	state.SetGlobal("now_ms", state.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(time.Now().UTC().UnixMilli()))
		return 1
	}))
	state.SetGlobal("format_date", state.NewFunction(func(l *lua.LState) int {
		ms := int64(l.CheckNumber(1))
		layout := l.OptString(2, "2006-01-02")
		l.Push(lua.LString(time.UnixMilli(ms).UTC().Format(layout)))
		return 1
	}))
	state.SetGlobal("same_day", state.NewFunction(func(l *lua.LState) int {
		left := time.UnixMilli(int64(l.CheckNumber(1))).UTC()
		right := time.UnixMilli(int64(l.CheckNumber(2))).UTC()
		l.Push(lua.LBool(left.Year() == right.Year() && left.YearDay() == right.YearDay()))
		return 1
	}))
}

func luaDecimalBinary(l *lua.LState, fn func(decimal.Decimal, decimal.Decimal) (decimal.Decimal, error)) int {
	left, err := luaDecimalArg(l, 1)
	if err != nil {
		l.RaiseError("%s", err.Error())
		return 0
	}
	right, err := luaDecimalArg(l, 2)
	if err != nil {
		l.RaiseError("%s", err.Error())
		return 0
	}
	result, err := fn(left, right)
	if err != nil {
		l.RaiseError("%s", err.Error())
		return 0
	}
	l.Push(lua.LString(result.String()))
	return 1
}

func luaDecimalArg(l *lua.LState, index int) (decimal.Decimal, error) {
	value := l.CheckAny(index)
	switch typed := value.(type) {
	case lua.LString:
		return decimal.Parse(string(typed))
	case lua.LNumber:
		return decimal.Parse(strconv.FormatFloat(float64(typed), 'f', -1, 64))
	default:
		return decimal.Parse(value.String())
	}
}

func enforceLuaOutputLimit(node marketing.WorkflowNode, output map[string]any) error {
	limit := int(workflowConfigNumber(workflowNodeConfigMap(node), "maxOutputBytes"))
	if limit <= 0 {
		limit = defaultLuaOutputMaxBytes
	}
	if limit > hardLuaOutputMaxBytes {
		limit = hardLuaOutputMaxBytes
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > limit {
		return fmt.Errorf("lua output exceeds maxOutputBytes: %d > %d", len(encoded), limit)
	}
	return nil
}

func luaGetPath(root lua.LValue, path string) lua.LValue {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return root
	}
	path = strings.TrimPrefix(path, "$.")
	current := root
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return lua.LNil
		}
		table, ok := current.(*lua.LTable)
		if !ok {
			return lua.LNil
		}
		if index, err := strconv.Atoi(part); err == nil {
			current = table.RawGetInt(index)
		} else {
			current = table.RawGetString(part)
		}
		if current == lua.LNil {
			return lua.LNil
		}
	}
	return current
}

func luaValueIsEmpty(value lua.LValue) bool {
	if value == lua.LNil {
		return true
	}
	switch typed := value.(type) {
	case lua.LBool:
		return false
	case lua.LNumber:
		return false
	case lua.LString:
		return strings.TrimSpace(string(typed)) == ""
	case *lua.LTable:
		empty := true
		typed.ForEach(func(lua.LValue, lua.LValue) {
			empty = false
		})
		return empty
	default:
		return value == lua.LNil
	}
}

func workflowLuaContext(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) map[string]any {
	executionContext, _ := ctx.Value(workflowNodeExecutionContextKey{}).(workflowNodeExecutionContext)
	return map[string]any{
		"event":    workflowJSONValue(runtime.Inbox.Event),
		"activity": workflowJSONValue(runtime.Activity),
		"runtime": map[string]any{
			"qualified":      runtime.Qualified,
			"reason":         runtime.Reason,
			"ruleResult":     workflowJSONValue(runtime.RuleResult),
			"participation":  workflowJSONValue(runtime.Participation),
			"grantOrders":    workflowJSONValue(runtime.GrantOrders),
			"outboxMessages": workflowJSONValue(runtime.OutboxMessages),
		},
		"node": map[string]any{
			"id":     node.ID,
			"type":   node.Type,
			"kind":   node.Kind,
			"code":   node.Code,
			"title":  node.Title,
			"config": workflowNodeConfigMap(node),
			"input":  executionContext.ResolvedInput,
		},
		"nodes": cloneWorkflowNodeOutputs(executionContext.NodeOutputs),
		"vars":  cloneWorkflowVars(executionContext.Variables),
	}
}

func cloneWorkflowNodeOutputs(in map[string]map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(in))
	for nodeID, output := range in {
		copied := make(map[string]any, len(output))
		for key, value := range output {
			copied[key] = workflowJSONValue(value)
		}
		out[nodeID] = copied
	}
	return out
}

func cloneWorkflowVars(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = workflowJSONValue(value)
	}
	return out
}

func workflowSetVar(vars map[string]any, scope string, key string, value any) {
	if vars == nil || strings.TrimSpace(key) == "" {
		return
	}
	scope = normalizeWorkflowVarScope(scope)
	value = workflowJSONValue(value)
	if scope == "execution" {
		vars[key] = value
	}
	namespace := workflowVarNamespace(vars, scope)
	namespace[key] = value
}

func workflowGetVar(vars map[string]any, scope string, key string) any {
	if vars == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	scope = normalizeWorkflowVarScope(scope)
	if scope == "execution" {
		if value, ok := vars[key]; ok {
			return workflowJSONValue(value)
		}
	}
	if namespace, ok := vars[scope].(map[string]any); ok {
		return workflowJSONValue(namespace[key])
	}
	return nil
}

func workflowVarNamespace(vars map[string]any, scope string) map[string]any {
	if existing, ok := vars[scope].(map[string]any); ok {
		return existing
	}
	namespace := map[string]any{}
	vars[scope] = namespace
	return namespace
}

func normalizeWorkflowVarScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "global", "workflow":
		return "execution"
	case "local":
		return "node"
	default:
		return strings.ToLower(strings.TrimSpace(scope))
	}
}

func goValueToLua(state *lua.LState, value any) lua.LValue {
	switch typed := workflowJSONValue(value).(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(typed)
	case float64:
		return lua.LNumber(typed)
	case string:
		return lua.LString(typed)
	case map[string]any:
		table := state.NewTable()
		for key, item := range typed {
			table.RawSetString(key, goValueToLua(state, item))
		}
		return table
	case []any:
		table := state.NewTable()
		for _, item := range typed {
			table.Append(goValueToLua(state, item))
		}
		return table
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return lua.LString(fmt.Sprint(typed))
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return lua.LString(fmt.Sprint(typed))
		}
		return goValueToLua(state, decoded)
	}
}

func luaValueToGo(value lua.LValue) any {
	switch value.Type() {
	case lua.LTNil:
		return nil
	case lua.LTBool:
		return bool(value.(lua.LBool))
	case lua.LTNumber:
		return float64(value.(lua.LNumber))
	case lua.LTString:
		return string(value.(lua.LString))
	case lua.LTTable:
		table := value.(*lua.LTable)
		if luaTableIsArray(table) {
			out := make([]any, 0, table.Len())
			for i := 1; i <= table.Len(); i++ {
				out = append(out, luaValueToGo(table.RawGetInt(i)))
			}
			return out
		}
		out := map[string]any{}
		table.ForEach(func(key lua.LValue, item lua.LValue) {
			out[key.String()] = luaValueToGo(item)
		})
		return out
	default:
		return value.String()
	}
}

func luaTableIsArray(table *lua.LTable) bool {
	length := table.Len()
	if length == 0 {
		return false
	}
	isArray := true
	count := 0
	table.ForEach(func(key lua.LValue, _ lua.LValue) {
		count++
		if number, ok := key.(lua.LNumber); !ok || int(number) < 1 || int(number) > length {
			isArray = false
		}
	})
	return isArray && count == length
}
