package core

import (
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
)

// buildBleveQuery converts a SearchQuery to a Bleve query.
// When Text is a non-empty string it is parsed as a query expression supporting:
//
//	tag:value     — match a tag
//	key:value     — match a frontmatter field value
//	word          — free-text search
//	AND / OR      — boolean operators (AND binds tighter than OR)
//	( ... )       — grouping
//
// Programmatic Tags and Filters fields are ANDed on top of the Text expression.
func buildBleveQuery(q SearchQuery) query.Query {
	var parts []query.Query

	if q.Text != "" {
		parts = append(parts, parseExpr(q.Text))
	}

	for _, tag := range q.Tags {
		tq := bleve.NewMatchQuery(tag)
		tq.SetField("tags")
		parts = append(parts, tq)
	}

	for k, v := range q.Filters {
		mq := bleve.NewMatchQuery(v)
		mq.SetField(k)
		parts = append(parts, mq)
	}

	switch len(parts) {
	case 0:
		return bleve.NewMatchAllQuery()
	case 1:
		return parts[0]
	default:
		bq := bleve.NewBooleanQuery()
		for _, p := range parts {
			bq.AddMust(p)
		}
		return bq
	}
}

// ---- Expression parser ----
//
// Grammar (EBNF):
//   expr    = orExpr
//   orExpr  = andExpr { 'OR' andExpr }
//   andExpr = atom { 'AND' atom }
//   atom    = '(' expr ')' | term
//   term    = FIELD_COLON_VALUE | WORD

type tokenKind int

const (
	tokWord tokenKind = iota
	tokFieldValue
	tokAnd
	tokOr
	tokLParen
	tokRParen
	tokEOF
)

type token struct {
	kind  tokenKind
	text  string
	field string // for tokFieldValue
	value string // for tokFieldValue
}

// tokenize splits a query expression into tokens.
func tokenize(expr string) []token {
	var tokens []token
	expr = strings.TrimSpace(expr)
	i := 0
	for i < len(expr) {
		// Skip whitespace.
		if expr[i] == ' ' || expr[i] == '\t' {
			i++
			continue
		}
		if expr[i] == '(' {
			tokens = append(tokens, token{kind: tokLParen, text: "("})
			i++
			continue
		}
		if expr[i] == ')' {
			tokens = append(tokens, token{kind: tokRParen, text: ")"})
			i++
			continue
		}
		// Collect a word until whitespace or parens.
		j := i
		for j < len(expr) && expr[j] != ' ' && expr[j] != '\t' && expr[j] != '(' && expr[j] != ')' {
			j++
		}
		word := expr[i:j]
		i = j

		switch strings.ToUpper(word) {
		case "AND":
			tokens = append(tokens, token{kind: tokAnd, text: word})
		case "OR":
			tokens = append(tokens, token{kind: tokOr, text: word})
		default:
			// Check for field:value syntax.
			if colon := strings.IndexByte(word, ':'); colon > 0 && colon < len(word)-1 {
				field := word[:colon]
				value := word[colon+1:]
				tokens = append(tokens, token{kind: tokFieldValue, text: word, field: field, value: value})
			} else {
				tokens = append(tokens, token{kind: tokWord, text: word})
			}
		}
	}
	tokens = append(tokens, token{kind: tokEOF})
	return tokens
}

// exprParser is a simple recursive-descent parser for search expressions.
type exprParser struct {
	tokens []token
	pos    int
}

func (p *exprParser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{kind: tokEOF}
	}
	return p.tokens[p.pos]
}

func (p *exprParser) consume() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *exprParser) parseOr() query.Query {
	left := p.parseAnd()
	for p.peek().kind == tokOr {
		p.consume() // consume OR
		right := p.parseAnd()
		bq := bleve.NewBooleanQuery()
		bq.AddShould(left)
		bq.AddShould(right)
		bq.SetMinShould(1)
		left = bq
	}
	return left
}

func (p *exprParser) parseAnd() query.Query {
	left := p.parseAtom()
	for p.peek().kind == tokAnd {
		p.consume() // consume AND
		right := p.parseAtom()
		bq := bleve.NewBooleanQuery()
		bq.AddMust(left)
		bq.AddMust(right)
		left = bq
	}
	return left
}

func (p *exprParser) parseAtom() query.Query {
	t := p.peek()
	if t.kind == tokLParen {
		p.consume() // consume (
		q := p.parseOr()
		if p.peek().kind == tokRParen {
			p.consume() // consume )
		}
		return q
	}
	return p.parseTerm()
}

func (p *exprParser) parseTerm() query.Query {
	t := p.consume()
	switch t.kind {
	case tokFieldValue:
		return termToQuery(t.field, t.value)
	case tokWord:
		mq := bleve.NewMatchQuery(t.text)
		mq.SetField("merged")
		return mq
	default:
		// Unexpected token — return match-all as fallback.
		return bleve.NewMatchAllQuery()
	}
}

// termToQuery converts a field:value token to an appropriate Bleve query.
func termToQuery(field, value string) query.Query {
	switch strings.ToLower(field) {
	case "tag", "tags":
		mq := bleve.NewMatchQuery(value)
		mq.SetField("tags")
		return mq
	default:
		// For arbitrary frontmatter fields (status, project, etc.) use a phrase query
		// on the fm field so "status:done" matches "status done" but not "status wip".
		mpq := bleve.NewMatchPhraseQuery(field + " " + value)
		mpq.SetField("fm")
		return mpq
	}
}

// parseExpr parses a raw query expression string into a Bleve query.
// Returns MatchAllQuery if expr is empty.
func parseExpr(expr string) query.Query {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return bleve.NewMatchAllQuery()
	}
	tokens := tokenize(expr)
	p := &exprParser{tokens: tokens}
	return p.parseOr()
}
