package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type serviceSchemaValidator struct {
	service *Service
}

func (v serviceSchemaValidator) validateSchemaDocument(raw json.RawMessage) error {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return errors.New("SERVICE_SCHEMA_FORMAT_INVALID:schemaJson must be json object")
	}
	return validateJSONSchemaSubset(schema, "$")
}

func (v serviceSchemaValidator) validatePayload(raw json.RawMessage, payload any) error {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return errors.New("SERVICE_SCHEMA_FORMAT_INVALID:schemaJson must be json object")
	}
	if err := validateJSONSchemaSubset(schema, "$"); err != nil {
		return err
	}
	return validateJSONValueAgainstSchema(schema, workflowJSONValue(payload), "$")
}

func (v serviceSchemaValidator) validateInterfaceSchemas(ctx context.Context, tenantID string, serviceInterface marketing.ServiceInterface) error {
	for _, ref := range []struct {
		value      string
		schemaType marketing.ServiceSchemaType
		field      string
	}{
		{serviceInterface.InputSchemaRef, marketing.ServiceSchemaInput, "inputSchemaRef"},
		{serviceInterface.OutputSchemaRef, marketing.ServiceSchemaOutput, "outputSchemaRef"},
		{serviceInterface.ErrorSchemaRef, marketing.ServiceSchemaError, "errorSchemaRef"},
	} {
		if strings.TrimSpace(ref.value) == "" {
			continue
		}
		schema, err := v.service.getActiveServiceSchema(ctx, tenantID, ref.value, ref.schemaType)
		if err != nil {
			return fmt.Errorf("SERVICE_SCHEMA_NOT_FOUND:%s:%s", ref.field, ref.value)
		}
		if err := v.validateSchemaDocument(schema.SchemaJSON); err != nil {
			return fmt.Errorf("%s:%s:%w", "SERVICE_SCHEMA_FORMAT_INVALID", ref.field, err)
		}
	}
	return nil
}

func (v serviceSchemaValidator) validateInterfaceInput(ctx context.Context, tenantID string, serviceInterface marketing.ServiceInterface, payload any) error {
	return v.validateInterfacePayload(ctx, tenantID, serviceInterface.InputSchemaRef, marketing.ServiceSchemaInput, payload, "INPUT")
}

func (v serviceSchemaValidator) validateInterfaceOutput(ctx context.Context, tenantID string, serviceInterface marketing.ServiceInterface, payload any) error {
	return v.validateInterfacePayload(ctx, tenantID, serviceInterface.OutputSchemaRef, marketing.ServiceSchemaOutput, payload, "OUTPUT")
}

func (v serviceSchemaValidator) validateInterfaceError(ctx context.Context, tenantID string, serviceInterface marketing.ServiceInterface, payload any) error {
	return v.validateInterfacePayload(ctx, tenantID, serviceInterface.ErrorSchemaRef, marketing.ServiceSchemaError, payload, "ERROR")
}

func (v serviceSchemaValidator) validateInterfacePayload(ctx context.Context, tenantID string, schemaRef string, schemaType marketing.ServiceSchemaType, payload any, phase string) error {
	if strings.TrimSpace(schemaRef) == "" {
		return nil
	}
	schema, err := v.service.getActiveServiceSchema(ctx, tenantID, schemaRef, schemaType)
	if err != nil {
		return fmt.Errorf("SERVICE_SCHEMA_NOT_FOUND:%s:%s", phase, schemaRef)
	}
	if err := v.validatePayload(schema.SchemaJSON, payload); err != nil {
		return fmt.Errorf("SERVICE_SCHEMA_%s_INVALID:%s:%w", phase, schemaRef, err)
	}
	return nil
}

func validateJSONSchemaSubset(schema map[string]any, path string) error {
	forbiddenKeywords := []string{"oneOf", "anyOf", "allOf", "not", "if", "then", "else", "$ref"}
	for _, keyword := range forbiddenKeywords {
		if _, ok := schema[keyword]; ok {
			return fmt.Errorf("%s.%s is not supported", path, keyword)
		}
	}
	if rawType, ok := schema["type"]; ok {
		if _, ok := rawType.(string); !ok {
			return fmt.Errorf("%s.type must be string", path)
		}
		if !jsonSchemaTypeSupported(rawType.(string)) {
			return fmt.Errorf("%s.type unsupported: %s", path, rawType.(string))
		}
	}
	if rawRequired, ok := schema["required"]; ok {
		items, ok := rawRequired.([]any)
		if !ok {
			return fmt.Errorf("%s.required must be array", path)
		}
		for index, item := range items {
			if strings.TrimSpace(fmt.Sprint(item)) == "" {
				return fmt.Errorf("%s.required[%d] must be string", path, index)
			}
		}
	}
	if rawProps, ok := schema["properties"]; ok {
		props, ok := rawProps.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.properties must be object", path)
		}
		for key, rawChild := range props {
			child, ok := rawChild.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.properties.%s must be object", path, key)
			}
			if err := validateJSONSchemaSubset(child, path+".properties."+key); err != nil {
				return err
			}
		}
	}
	if rawItems, ok := schema["items"]; ok {
		child, ok := rawItems.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.items must be object", path)
		}
		if err := validateJSONSchemaSubset(child, path+".items"); err != nil {
			return err
		}
	}
	if rawAdditional, ok := schema["additionalProperties"]; ok {
		if _, ok := rawAdditional.(bool); !ok {
			return fmt.Errorf("%s.additionalProperties must be boolean", path)
		}
	}
	if rawEnum, ok := schema["enum"]; ok {
		items, ok := rawEnum.([]any)
		if !ok || len(items) == 0 {
			return fmt.Errorf("%s.enum must be non-empty array", path)
		}
	}
	for _, keyword := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum"} {
		if raw, ok := schema[keyword]; ok {
			if _, ok := jsonSchemaNumber(raw); !ok {
				return fmt.Errorf("%s.%s must be number", path, keyword)
			}
		}
	}
	for _, keyword := range []string{"minLength", "maxLength"} {
		if raw, ok := schema[keyword]; ok {
			number, ok := jsonSchemaInteger(raw)
			if !ok || number < 0 {
				return fmt.Errorf("%s.%s must be non-negative integer", path, keyword)
			}
		}
	}
	if rawPattern, ok := schema["pattern"]; ok {
		pattern, ok := rawPattern.(string)
		if !ok {
			return fmt.Errorf("%s.pattern must be string", path)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%s.pattern is invalid: %w", path, err)
		}
	}
	if rawFormat, ok := schema["format"]; ok {
		format, ok := rawFormat.(string)
		if !ok {
			return fmt.Errorf("%s.format must be string", path)
		}
		if !jsonSchemaFormatSupported(format) {
			return fmt.Errorf("%s.format unsupported: %s", path, format)
		}
	}
	return nil
}

func jsonSchemaTypeSupported(value string) bool {
	switch strings.TrimSpace(value) {
	case "object", "array", "string", "number", "integer", "boolean", "null":
		return true
	default:
		return false
	}
}

func validateJSONValueAgainstSchema(schema map[string]any, value any, path string) error {
	if rawType, ok := schema["type"].(string); ok {
		if !jsonValueMatchesType(value, rawType) {
			return fmt.Errorf("%s expected %s", path, rawType)
		}
	}
	if rawEnum, ok := schema["enum"].([]any); ok && len(rawEnum) > 0 {
		matched := false
		for _, item := range rawEnum {
			if jsonSchemaValuesEqual(value, item) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s must be one of enum values", path)
		}
	}
	if number, ok := jsonSchemaNumber(value); ok {
		if minimum, ok := jsonSchemaNumber(schema["minimum"]); ok && number < minimum {
			return fmt.Errorf("%s must be >= %v", path, minimum)
		}
		if maximum, ok := jsonSchemaNumber(schema["maximum"]); ok && number > maximum {
			return fmt.Errorf("%s must be <= %v", path, maximum)
		}
		if minimum, ok := jsonSchemaNumber(schema["exclusiveMinimum"]); ok && number <= minimum {
			return fmt.Errorf("%s must be > %v", path, minimum)
		}
		if maximum, ok := jsonSchemaNumber(schema["exclusiveMaximum"]); ok && number >= maximum {
			return fmt.Errorf("%s must be < %v", path, maximum)
		}
	}
	if text, ok := value.(string); ok {
		if minLength, ok := jsonSchemaInteger(schema["minLength"]); ok && len([]rune(text)) < minLength {
			return fmt.Errorf("%s length must be >= %d", path, minLength)
		}
		if maxLength, ok := jsonSchemaInteger(schema["maxLength"]); ok && len([]rune(text)) > maxLength {
			return fmt.Errorf("%s length must be <= %d", path, maxLength)
		}
		if pattern, ok := schema["pattern"].(string); ok {
			matched, err := regexp.MatchString(pattern, text)
			if err != nil {
				return fmt.Errorf("%s pattern is invalid: %w", path, err)
			}
			if !matched {
				return fmt.Errorf("%s does not match pattern", path)
			}
		}
		if format, ok := schema["format"].(string); ok && !jsonSchemaFormatMatches(text, format) {
			return fmt.Errorf("%s does not match format %s", path, format)
		}
	}
	if required := jsonSchemaRequired(schema); len(required) > 0 {
		mapped, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s expected object for required fields", path)
		}
		for _, key := range required {
			if _, ok := mapped[key]; !ok {
				return fmt.Errorf("%s.%s is required", path, key)
			}
		}
	}
	if rawProps, ok := schema["properties"].(map[string]any); ok {
		mapped, ok := value.(map[string]any)
		if !ok {
			if schema["type"] == nil {
				return fmt.Errorf("%s expected object for properties", path)
			}
			return nil
		}
		allowed := map[string]struct{}{}
		for key, rawChild := range rawProps {
			allowed[key] = struct{}{}
			child, _ := rawChild.(map[string]any)
			if child == nil {
				continue
			}
			if childValue, ok := mapped[key]; ok {
				if err := validateJSONValueAgainstSchema(child, childValue, path+"."+key); err != nil {
					return err
				}
			}
		}
		if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
			keys := make([]string, 0, len(mapped))
			for key := range mapped {
				if _, ok := allowed[key]; !ok {
					keys = append(keys, key)
				}
			}
			if len(keys) > 0 {
				sort.Strings(keys)
				return fmt.Errorf("%s additional properties not allowed: %s", path, strings.Join(keys, ","))
			}
		}
	}
	if rawItems, ok := schema["items"].(map[string]any); ok {
		items, ok := value.([]any)
		if !ok {
			if schema["type"] == nil {
				return fmt.Errorf("%s expected array for items", path)
			}
			return nil
		}
		for index, item := range items {
			if err := validateJSONValueAgainstSchema(rawItems, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func jsonSchemaRequired(schema map[string]any) []string {
	rawRequired, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rawRequired))
	for _, item := range rawRequired {
		if key, ok := item.(string); ok && strings.TrimSpace(key) != "" {
			out = append(out, key)
		}
	}
	return out
}

func jsonValueMatchesType(value any, schemaType string) bool {
	switch schemaType {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32, json.Number:
			return true
		default:
			return false
		}
	case "integer":
		switch typed := value.(type) {
		case int, int64, int32:
			return true
		case float64:
			return typed == float64(int64(typed))
		case json.Number:
			_, err := typed.Int64()
			return err == nil
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return true
	}
}

func serviceSchemaCompatible(previous marketing.ServiceSchema, next marketing.ServiceSchema) error {
	var previousSchema map[string]any
	var nextSchema map[string]any
	if err := json.Unmarshal(previous.SchemaJSON, &previousSchema); err != nil {
		return err
	}
	if err := json.Unmarshal(next.SchemaJSON, &nextSchema); err != nil {
		return err
	}
	if fmt.Sprint(previousSchema["type"]) != fmt.Sprint(nextSchema["type"]) {
		return fmt.Errorf("top-level type changed from %s to %s", previousSchema["type"], nextSchema["type"])
	}
	for _, key := range jsonSchemaRequired(previousSchema) {
		if !stringSliceContains(jsonSchemaRequired(nextSchema), key) {
			return fmt.Errorf("required field removed: %s", key)
		}
	}
	previousProps, _ := previousSchema["properties"].(map[string]any)
	nextProps, _ := nextSchema["properties"].(map[string]any)
	for key, rawPrevious := range previousProps {
		rawNext, ok := nextProps[key]
		if !ok {
			return fmt.Errorf("property removed: %s", key)
		}
		previousChild, _ := rawPrevious.(map[string]any)
		nextChild, _ := rawNext.(map[string]any)
		if previousChild == nil || nextChild == nil {
			continue
		}
		if fmt.Sprint(previousChild["type"]) != fmt.Sprint(nextChild["type"]) {
			return fmt.Errorf("property type changed: %s", key)
		}
	}
	return nil
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func jsonSchemaNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func jsonSchemaInteger(value any) (int, bool) {
	number, ok := jsonSchemaNumber(value)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func jsonSchemaValuesEqual(left any, right any) bool {
	if leftNumber, ok := jsonSchemaNumber(left); ok {
		if rightNumber, ok := jsonSchemaNumber(right); ok {
			return leftNumber == rightNumber
		}
	}
	leftJSON, _ := json.Marshal(workflowJSONValue(left))
	rightJSON, _ := json.Marshal(workflowJSONValue(right))
	return string(leftJSON) == string(rightJSON)
}

func jsonSchemaFormatSupported(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "date-time", "date", "email", "uri", "uuid":
		return true
	default:
		return false
	}
}

func jsonSchemaFormatMatches(value string, format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "date-time":
		_, err := time.Parse(time.RFC3339, value)
		return err == nil
	case "date":
		_, err := time.Parse("2006-01-02", value)
		return err == nil
	case "email":
		_, err := mail.ParseAddress(value)
		return err == nil
	case "uri":
		parsed, err := url.ParseRequestURI(value)
		return err == nil && parsed.Scheme != ""
	case "uuid":
		return regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`).MatchString(value)
	default:
		return true
	}
}

func jsonSchemaCoversPath(raw json.RawMessage, path string) error {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return errors.New("schemaJson must be json object")
	}
	if err := validateJSONSchemaSubset(schema, "$"); err != nil {
		return err
	}
	return jsonSchemaMapCoversPath(schema, path)
}

func jsonSchemaMapCoversPath(schema map[string]any, path string) error {
	segments := normalizeJSONPathSegments(path)
	if len(segments) == 0 {
		return nil
	}
	current := schema
	for _, segment := range segments {
		if rawItems, ok := current["items"].(map[string]any); ok && segment == "[]" {
			current = rawItems
			continue
		}
		props, _ := current["properties"].(map[string]any)
		rawChild, ok := props[segment]
		if !ok {
			if additional, ok := current["additionalProperties"].(bool); ok && additional {
				return nil
			}
			return fmt.Errorf("schema does not cover path %s", path)
		}
		child, ok := rawChild.(map[string]any)
		if !ok {
			return fmt.Errorf("schema path %s is not object", path)
		}
		current = child
	}
	return nil
}

func normalizeJSONPathSegments(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return nil
	}
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return nil
	}
	raw := strings.Split(path, ".")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		for strings.Contains(item, "[") {
			prefix := item[:strings.Index(item, "[")]
			if prefix != "" {
				out = append(out, prefix)
			}
			out = append(out, "[]")
			close := strings.Index(item, "]")
			if close < 0 || close+1 >= len(item) {
				item = ""
				break
			}
			item = item[close+1:]
		}
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
