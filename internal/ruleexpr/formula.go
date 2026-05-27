package ruleexpr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/ex-stack/ExMarketing/internal/decimal"
)

const (
	maxFormulaLength = 512
	maxFormulaFields = 20
	maxFormulaDepth  = 8
)

type Kind string

const (
	KindDecimal Kind = "DECIMAL"
	KindBoolean Kind = "BOOLEAN"
	KindString  Kind = "STRING"
)

type Value struct {
	Kind    Kind
	Decimal decimal.Decimal
	Boolean bool
	String  string
}

type FieldResolver func(path string) (any, bool)
type FieldValidator func(path string) bool

type Expr interface {
	Eval(resolve FieldResolver) (Value, error)
	Fields() []string
}

func Parse(input string, validate FieldValidator) (Expr, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return nil, err
	}
	parser := formulaParser{tokens: tokens, validate: validate}
	expr, err := parser.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if parser.peek().kind != tokenEOF {
		return nil, fmt.Errorf("unexpected token %q", parser.peek().text)
	}
	if len(uniqueFields(expr.Fields())) > maxFormulaFields {
		return nil, errors.New("formula references too many fields")
	}
	return expr, nil
}

func SupportedField(field string, pluginCode string) bool {
	field = strings.TrimSpace(field)
	if strings.HasPrefix(field, "attributes.") && len(field) > len("attributes.") {
		return true
	}
	if supportedStateField(field, pluginCode) {
		return true
	}
	if strings.HasPrefix(field, "facts.") {
		rest := strings.TrimPrefix(field, "facts.")
		parts := strings.SplitN(rest, ".", 2)
		return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
	}
	switch field {
	case "eventId", "eventType", "sourceSystem", "tenantId", "occurredAt", "subject.type", "subject.id", "biz.bizType", "biz.bizId":
		return true
	default:
		return false
	}
}

func supportedStateField(field string, pluginCode string) bool {
	for _, prefix := range []string{"state.activity.", "state.subject.", "state.quota.", "state.exclusive."} {
		if strings.HasPrefix(field, prefix) && len(field) > len(prefix) {
			return true
		}
	}
	pluginCode = strings.TrimSpace(pluginCode)
	if pluginCode == "" {
		return false
	}
	prefix := "state.plugin." + pluginCode + "."
	return strings.HasPrefix(field, prefix) && len(field) > len(prefix)
}

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenIdent
	tokenNumber
	tokenString
	tokenBool
	tokenPlus
	tokenMinus
	tokenStar
	tokenSlash
	tokenLParen
	tokenRParen
)

type token struct {
	kind tokenKind
	text string
}

func tokenize(input string) ([]token, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("formula is required")
	}
	if len(input) > maxFormulaLength {
		return nil, errors.New("formula is too long")
	}
	tokens := []token{}
	for i := 0; i < len(input); {
		r := rune(input[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		switch input[i] {
		case '+':
			tokens = append(tokens, token{kind: tokenPlus, text: "+"})
			i++
		case '-':
			tokens = append(tokens, token{kind: tokenMinus, text: "-"})
			i++
		case '*':
			tokens = append(tokens, token{kind: tokenStar, text: "*"})
			i++
		case '/':
			tokens = append(tokens, token{kind: tokenSlash, text: "/"})
			i++
		case '(':
			tokens = append(tokens, token{kind: tokenLParen, text: "("})
			i++
		case ')':
			tokens = append(tokens, token{kind: tokenRParen, text: ")"})
			i++
		case '"':
			text, next, err := readString(input, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenString, text: text})
			i = next
		default:
			if isIdentStart(input[i]) {
				text, next := readIdent(input, i)
				upper := strings.ToUpper(text)
				if upper == "TRUE" || upper == "FALSE" {
					tokens = append(tokens, token{kind: tokenBool, text: upper})
				} else {
					tokens = append(tokens, token{kind: tokenIdent, text: text})
				}
				i = next
				continue
			}
			if isDigit(input[i]) || input[i] == '.' {
				text, next := readNumber(input, i)
				if _, err := decimal.Parse(text); err != nil {
					return nil, fmt.Errorf("invalid decimal literal %q", text)
				}
				tokens = append(tokens, token{kind: tokenNumber, text: text})
				i = next
				continue
			}
			return nil, fmt.Errorf("unsupported formula token %q", string(input[i]))
		}
	}
	tokens = append(tokens, token{kind: tokenEOF})
	return tokens, nil
}

func readString(input string, start int) (string, int, error) {
	for i := start + 1; i < len(input); i++ {
		if input[i] == '\\' {
			i++
			continue
		}
		if input[i] == '"' {
			var out string
			if err := json.Unmarshal([]byte(input[start:i+1]), &out); err != nil {
				return "", 0, fmt.Errorf("invalid string literal")
			}
			return out, i + 1, nil
		}
	}
	return "", 0, errors.New("unterminated string literal")
}

func readIdent(input string, start int) (string, int) {
	i := start + 1
	for i < len(input) && (isIdentPart(input[i]) || input[i] == '.') {
		i++
	}
	return input[start:i], i
}

func readNumber(input string, start int) (string, int) {
	i := start
	for i < len(input) && (isDigit(input[i]) || input[i] == '.') {
		i++
	}
	return input[start:i], i
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || isDigit(b)
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

type formulaParser struct {
	tokens   []token
	pos      int
	validate FieldValidator
}

func (p *formulaParser) parseExpression(depth int) (Expr, error) {
	if depth > maxFormulaDepth {
		return nil, errors.New("formula nesting is too deep")
	}
	return p.parseAddSub(depth)
}

func (p *formulaParser) parseAddSub(depth int) (Expr, error) {
	left, err := p.parseMulDiv(depth)
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenPlus || p.peek().kind == tokenMinus {
		op := p.next().text
		right, err := p.parseMulDiv(depth)
		if err != nil {
			return nil, err
		}
		left = binaryExpr{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *formulaParser) parseMulDiv(depth int) (Expr, error) {
	left, err := p.parseUnary(depth)
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenStar || p.peek().kind == tokenSlash {
		op := p.next().text
		right, err := p.parseUnary(depth)
		if err != nil {
			return nil, err
		}
		left = binaryExpr{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *formulaParser) parseUnary(depth int) (Expr, error) {
	if p.peek().kind == tokenMinus {
		p.next()
		child, err := p.parseUnary(depth)
		if err != nil {
			return nil, err
		}
		return unaryExpr{op: "-", child: child}, nil
	}
	return p.parsePrimary(depth)
}

func (p *formulaParser) parsePrimary(depth int) (Expr, error) {
	tok := p.next()
	switch tok.kind {
	case tokenNumber:
		value, _ := decimal.Parse(tok.text)
		return literalExpr{value: Value{Kind: KindDecimal, Decimal: value}}, nil
	case tokenString:
		return literalExpr{value: Value{Kind: KindString, String: tok.text}}, nil
	case tokenBool:
		return literalExpr{value: Value{Kind: KindBoolean, Boolean: tok.text == "TRUE"}}, nil
	case tokenIdent:
		if p.validate != nil && !p.validate(tok.text) {
			return nil, fmt.Errorf("field %s is unsupported", tok.text)
		}
		return fieldExpr{path: tok.text}, nil
	case tokenLParen:
		expr, err := p.parseExpression(depth + 1)
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokenRParen {
			return nil, errors.New("missing closing parenthesis")
		}
		p.next()
		return expr, nil
	default:
		return nil, fmt.Errorf("unexpected token %q", tok.text)
	}
}

func (p *formulaParser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{kind: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *formulaParser) next() token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

type literalExpr struct {
	value Value
}

func (e literalExpr) Eval(FieldResolver) (Value, error) {
	return e.value, nil
}

func (e literalExpr) Fields() []string {
	return nil
}

type fieldExpr struct {
	path string
}

func (e fieldExpr) Eval(resolve FieldResolver) (Value, error) {
	raw, ok := resolve(e.path)
	if !ok {
		return Value{}, fmt.Errorf("field %s not found", e.path)
	}
	value, ok := scalarValue(raw)
	if !ok {
		return Value{}, fmt.Errorf("field %s has unsupported type", e.path)
	}
	return value, nil
}

func (e fieldExpr) Fields() []string {
	return []string{e.path}
}

type unaryExpr struct {
	op    string
	child Expr
}

func (e unaryExpr) Eval(resolve FieldResolver) (Value, error) {
	child, err := e.child.Eval(resolve)
	if err != nil {
		return Value{}, err
	}
	if child.Kind != KindDecimal {
		return Value{}, errors.New("unary minus requires decimal")
	}
	return Value{Kind: KindDecimal, Decimal: decimal.Zero().Sub(child.Decimal)}, nil
}

func (e unaryExpr) Fields() []string {
	return e.child.Fields()
}

type binaryExpr struct {
	op          string
	left, right Expr
}

func (e binaryExpr) Eval(resolve FieldResolver) (Value, error) {
	left, err := e.left.Eval(resolve)
	if err != nil {
		return Value{}, err
	}
	right, err := e.right.Eval(resolve)
	if err != nil {
		return Value{}, err
	}
	if left.Kind != KindDecimal || right.Kind != KindDecimal {
		return Value{}, fmt.Errorf("operator %s requires decimal operands", e.op)
	}
	switch e.op {
	case "+":
		return Value{Kind: KindDecimal, Decimal: left.Decimal.Add(right.Decimal)}, nil
	case "-":
		return Value{Kind: KindDecimal, Decimal: left.Decimal.Sub(right.Decimal)}, nil
	case "*":
		return Value{Kind: KindDecimal, Decimal: left.Decimal.Mul(right.Decimal)}, nil
	case "/":
		out, err := left.Decimal.Div(right.Decimal)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindDecimal, Decimal: out}, nil
	default:
		return Value{}, fmt.Errorf("unsupported operator %s", e.op)
	}
}

func (e binaryExpr) Fields() []string {
	return append(e.left.Fields(), e.right.Fields()...)
}

func scalarValue(value any) (Value, bool) {
	switch typed := value.(type) {
	case bool:
		return Value{Kind: KindBoolean, Boolean: typed}, true
	case string:
		if d, err := decimal.Parse(typed); err == nil {
			return Value{Kind: KindDecimal, Decimal: d, String: typed}, true
		}
		return Value{Kind: KindString, String: typed}, true
	case json.Number:
		d, err := decimal.Parse(typed.String())
		return Value{Kind: KindDecimal, Decimal: d, String: typed.String()}, err == nil
	case int:
		d, err := decimal.Parse(strconv.Itoa(typed))
		return Value{Kind: KindDecimal, Decimal: d, String: strconv.Itoa(typed)}, err == nil
	case int64:
		text := strconv.FormatInt(typed, 10)
		d, err := decimal.Parse(text)
		return Value{Kind: KindDecimal, Decimal: d, String: text}, err == nil
	default:
		return Value{}, false
	}
}

func uniqueFields(fields []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, field := range fields {
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}
