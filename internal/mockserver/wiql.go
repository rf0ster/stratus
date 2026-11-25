package mockserver

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// This file implements the subset of WIQL the mock server understands:
//
//	SELECT [field], ... FROM WorkItems
//	[WHERE cond {AND|OR cond}]          (parentheses allowed)
//	[ORDER BY [field] [ASC|DESC], ...]
//
// where cond is [field] op value and op is one of =, <>, !=, <, <=, >, >=,
// [NOT] IN (...), [NOT] UNDER, [NOT] CONTAINS. Values are 'strings',
// numbers, @Me or @project.

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokField
	tokString
	tokNumber
	tokMacro
	tokIdent
	tokOp
	tokPunct
)

type token struct {
	kind tokenKind
	text string
}

func tokenize(src string) ([]token, error) {
	var toks []token
	rs := []rune(src)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '[':
			end := indexRune(rs, i+1, ']')
			if end < 0 {
				return nil, fmt.Errorf("unterminated field reference")
			}
			toks = append(toks, token{tokField, strings.TrimSpace(string(rs[i+1 : end]))})
			i = end + 1
		case r == '\'':
			var b strings.Builder
			j := i + 1
			for ; j < len(rs); j++ {
				if rs[j] == '\'' {
					if j+1 < len(rs) && rs[j+1] == '\'' {
						b.WriteRune('\'')
						j++
						continue
					}
					break
				}
				b.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unterminated string literal")
			}
			toks = append(toks, token{tokString, b.String()})
			i = j + 1
		case r == '@':
			j := i + 1
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
				j++
			}
			toks = append(toks, token{tokMacro, strings.ToLower(string(rs[i+1 : j]))})
			i = j
		case unicode.IsDigit(r) || (r == '-' && i+1 < len(rs) && unicode.IsDigit(rs[i+1])):
			j := i + 1
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			toks = append(toks, token{tokNumber, string(rs[i:j])})
			i = j
		case unicode.IsLetter(r):
			j := i + 1
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
				j++
			}
			toks = append(toks, token{tokIdent, strings.ToUpper(string(rs[i:j]))})
			i = j
		case strings.ContainsRune("(),", r):
			toks = append(toks, token{tokPunct, string(r)})
			i++
		case strings.ContainsRune("=<>!", r):
			j := i + 1
			if j < len(rs) && strings.ContainsRune("=>", rs[j]) {
				j++
			}
			op := string(rs[i:j])
			if op == "!" {
				return nil, fmt.Errorf("unexpected character %q", r)
			}
			toks = append(toks, token{tokOp, op})
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q", r)
		}
	}
	return append(toks, token{kind: tokEOF}), nil
}

func indexRune(rs []rune, from int, r rune) int {
	for i := from; i < len(rs); i++ {
		if rs[i] == r {
			return i
		}
	}
	return -1
}

// value is a literal or macro on the right-hand side of a condition.
type value struct {
	text  string
	macro string // lower-case macro name without '@', or ""
}

type orderKey struct {
	field string
	desc  bool
}

type wiqlQuery struct {
	where expr // nil means match everything
	order []orderKey
}

type expr interface {
	eval(ctx *evalContext, it *item) bool
}

type andExpr struct{ left, right expr }
type orExpr struct{ left, right expr }

func (e andExpr) eval(ctx *evalContext, it *item) bool {
	return e.left.eval(ctx, it) && e.right.eval(ctx, it)
}

func (e orExpr) eval(ctx *evalContext, it *item) bool {
	return e.left.eval(ctx, it) || e.right.eval(ctx, it)
}

type condition struct {
	field  string
	op     string // =, <>, <, <=, >, >=, IN, UNDER, CONTAINS
	negate bool
	values []value
}

type parser struct {
	toks  []token
	pos   int
	field func(string) (string, bool) // resolves a field name to its canonical form
}

func parseWIQL(src string, resolveField func(string) (string, bool)) (*wiqlQuery, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, field: resolveField}
	return p.query()
}

func (p *parser) peek() token { return p.toks[p.pos] }

func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *parser) keyword(words ...string) bool {
	for i, w := range words {
		t := p.toks[min(p.pos+i, len(p.toks)-1)]
		if t.kind != tokIdent || t.text != w {
			return false
		}
	}
	p.pos += len(words)
	return true
}

func (p *parser) expect(words ...string) error {
	if !p.keyword(words...) {
		return fmt.Errorf("expected %s near %q", strings.Join(words, " "), p.peek().text)
	}
	return nil
}

func (p *parser) fieldRef() (string, error) {
	t := p.next()
	if t.kind != tokField {
		return "", fmt.Errorf("expected a [field] reference near %q", t.text)
	}
	name, ok := p.field(t.text)
	if !ok {
		return "", fmt.Errorf("TF51005: The query references a field that does not exist. The error is caused by «[%s]».", t.text)
	}
	return name, nil
}

func (p *parser) query() (*wiqlQuery, error) {
	if err := p.expect("SELECT"); err != nil {
		return nil, err
	}
	for {
		if _, err := p.fieldRef(); err != nil {
			return nil, err
		}
		if p.peek().text != "," {
			break
		}
		p.next()
	}
	if err := p.expect("FROM"); err != nil {
		return nil, err
	}
	if from := p.next(); from.kind != tokIdent || from.text != "WORKITEMS" {
		return nil, fmt.Errorf("the mock server only supports flat queries (FROM WorkItems)")
	}

	q := &wiqlQuery{}
	if p.keyword("WHERE") {
		where, err := p.or()
		if err != nil {
			return nil, err
		}
		q.where = where
	}
	if p.keyword("ORDER", "BY") {
		for {
			f, err := p.fieldRef()
			if err != nil {
				return nil, err
			}
			key := orderKey{field: f}
			if p.keyword("DESC") {
				key.desc = true
			} else {
				p.keyword("ASC")
			}
			q.order = append(q.order, key)
			if p.peek().text != "," {
				break
			}
			p.next()
		}
	}
	if t := p.peek(); t.kind != tokEOF {
		return nil, fmt.Errorf("unexpected %q", t.text)
	}
	return q, nil
}

func (p *parser) or() (expr, error) {
	left, err := p.and()
	if err != nil {
		return nil, err
	}
	for p.keyword("OR") {
		right, err := p.and()
		if err != nil {
			return nil, err
		}
		left = orExpr{left, right}
	}
	return left, nil
}

func (p *parser) and() (expr, error) {
	left, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.keyword("AND") {
		right, err := p.primary()
		if err != nil {
			return nil, err
		}
		left = andExpr{left, right}
	}
	return left, nil
}

func (p *parser) primary() (expr, error) {
	if p.peek().text == "(" && p.peek().kind == tokPunct {
		p.next()
		e, err := p.or()
		if err != nil {
			return nil, err
		}
		if t := p.next(); t.text != ")" {
			return nil, fmt.Errorf("expected ) near %q", t.text)
		}
		return e, nil
	}
	return p.condition()
}

func (p *parser) condition() (expr, error) {
	f, err := p.fieldRef()
	if err != nil {
		return nil, err
	}
	c := &condition{field: f}

	if t := p.peek(); t.kind == tokOp {
		p.next()
		c.op = t.text
		switch c.op {
		case "!=":
			c.op = "<>"
		case "=", "<>", "<", "<=", ">", ">=":
		default:
			return nil, fmt.Errorf("unsupported operator %q", t.text)
		}
	} else {
		c.negate = p.keyword("NOT")
		switch {
		case p.keyword("IN"):
			c.op = "IN"
		case p.keyword("UNDER"):
			c.op = "UNDER"
		case p.keyword("CONTAINS"):
			c.op = "CONTAINS"
		default:
			return nil, fmt.Errorf("expected an operator after [%s]", f)
		}
	}

	if c.op == "IN" {
		if t := p.next(); t.text != "(" {
			return nil, fmt.Errorf("expected ( after IN")
		}
		for {
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			c.values = append(c.values, v)
			t := p.next()
			if t.text == ")" {
				break
			}
			if t.text != "," {
				return nil, fmt.Errorf("expected , or ) in IN list near %q", t.text)
			}
		}
		return c, nil
	}

	v, err := p.value()
	if err != nil {
		return nil, err
	}
	c.values = []value{v}
	return c, nil
}

func (p *parser) value() (value, error) {
	t := p.next()
	switch t.kind {
	case tokString, tokNumber:
		return value{text: t.text}, nil
	case tokMacro:
		switch t.text {
		case "me", "project":
			return value{macro: t.text}, nil
		}
		return value{}, fmt.Errorf("the mock server doesn't support the @%s macro", t.text)
	}
	return value{}, fmt.Errorf("expected a value near %q", t.text)
}

// evalContext resolves macros while evaluating a query.
type evalContext struct {
	me      identity
	project string
}

func (c *condition) eval(ctx *evalContext, it *item) bool {
	match := false
	switch c.op {
	case "IN":
		for _, v := range c.values {
			if equals(ctx, it, c.field, v) {
				match = true
				break
			}
		}
	case "=":
		match = equals(ctx, it, c.field, c.values[0])
	case "<>":
		match = !equals(ctx, it, c.field, c.values[0])
	case "UNDER":
		path := strings.ToLower(fieldText(it, c.field))
		root := strings.ToLower(resolve(ctx, c.values[0]))
		match = path == root || strings.HasPrefix(path, root+`\`)
	case "CONTAINS":
		match = strings.Contains(strings.ToLower(fieldText(it, c.field)), strings.ToLower(resolve(ctx, c.values[0])))
	default:
		cmp := compareValues(it.fields[c.field], resolve(ctx, c.values[0]))
		switch c.op {
		case "<":
			match = cmp < 0
		case "<=":
			match = cmp <= 0
		case ">":
			match = cmp > 0
		case ">=":
			match = cmp >= 0
		}
	}
	return match != c.negate
}

func resolve(ctx *evalContext, v value) string {
	switch v.macro {
	case "me":
		return ctx.me.UniqueName
	case "project":
		return ctx.project
	}
	return v.text
}

func equals(ctx *evalContext, it *item, field string, v value) bool {
	want := resolve(ctx, v)
	switch got := it.fields[field].(type) {
	case nil:
		return want == ""
	case identity:
		return got.matches(want)
	case float64:
		n, err := strconv.ParseFloat(want, 64)
		return err == nil && n == got
	default:
		return strings.EqualFold(fieldText(it, field), want)
	}
}

// compareValues orders a field value against a literal: numerically when
// both are numbers, otherwise case-insensitively as text.
func compareValues(got any, want string) int {
	if n, ok := got.(float64); ok {
		if w, err := strconv.ParseFloat(want, 64); err == nil {
			switch {
			case n < w:
				return -1
			case n > w:
				return 1
			}
			return 0
		}
	}
	return strings.Compare(strings.ToLower(textOf(got)), strings.ToLower(want))
}

func fieldText(it *item, field string) string { return textOf(it.fields[field]) }

func textOf(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case identity:
		return v.DisplayName
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	}
	return fmt.Sprint(v)
}

// run returns the IDs of matching items in query order.
func (q *wiqlQuery) run(ctx *evalContext, items []*item) []int {
	var matched []*item
	for _, it := range items {
		if q.where == nil || q.where.eval(ctx, it) {
			matched = append(matched, it)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		for _, key := range q.order {
			a, b := matched[i].fields[key.field], matched[j].fields[key.field]
			cmp := compareValues(a, textOf(b))
			// Missing values sort first, like Azure DevOps.
			if a == nil && b != nil {
				cmp = -1
			} else if a != nil && b == nil {
				cmp = 1
			}
			if cmp != 0 {
				return (cmp < 0) != key.desc
			}
		}
		return false
	})
	ids := make([]int, len(matched))
	for i, it := range matched {
		ids[i] = it.id
	}
	return ids
}
