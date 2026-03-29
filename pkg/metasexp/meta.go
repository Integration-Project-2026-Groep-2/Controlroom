package metasexp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type TokenType byte

const (
	LPAREN TokenType = iota
	RPAREN
	KEYWORD
	ATOM
)

// Node is a single node in the AST.
// Next is the next sibling, First/Last are the first/last child, Parent is the enclosing node.
type Node struct {
	Next   *Node
	Parent *Node
	First  *Node
	Last   *Node
}

// AST holds the root of the parsed syntax tree and a cursor used during parsing.
// Abstract syntax tree
type AST struct {
	Root    *Node
	Current *Node
}

// Token is a single lexical unit produced by MetaLexer.
type Token struct {
	Type   TokenType
	Lexeme []byte
}

// MetaLexer holds the raw SEXP byte stream, the token list, and the read position.
type MetaLexer struct {
	Stream   []byte
	Tokens   []Token
	Position int
}

// TODO(nasr): because OOP is horrible and linking funcitons / methods with objects is more of a constraint than something else
// i cant use a generic thing the oop solution would be using an interface but i think i will change it to a more procedural type of programming in the future
// LoadFile reads the SEXP file at path/file into the lexer stream.
func (ml *MetaLexer) LoadFile(path string, file string) error {
	content, err := os.ReadFile(filepath.Join(path, file))
	if err != nil {
		return &MetaError{
			FileName: file,
			Content:  fmt.Sprintf("failed to open file >>> %v <<<", err),
		}
	}
	ml.Stream = content
	ml.Position = 0
	return nil
}

func (ml *MetaLexer) current() byte {
	return ml.Stream[ml.Position]
}

func (ml *MetaLexer) atEnd() bool {
	return ml.Position >= len(ml.Stream)
}

func (ml *MetaLexer) advance() {
	if !ml.atEnd() {
		ml.Position++
	}
}

func (ml *MetaLexer) peek() byte {
	if ml.Position+1 < len(ml.Stream) {
		return ml.Stream[ml.Position+1]
	}
	return 0
}

// IsSexp reports whether fileName has the ".sexp" extension.
func IsSexp(fileName string) bool {
	return filepath.Ext(fileName) == ".sexp"
}

func PushNode(tok Token, ast *AST) error {
	if token != nil {
		return fmt.Errorf("empty token")
	}

	AST.Current.Next.Lexeme = tok
	AST.Parent.Lexeme = tok
}

func (ml *MetaLexer) consume() (Token, error) {

	start := ml.Position

	for ml.current() != ' ' || ml.current() != ')' {

		ml.advance()
	}

	end := ml.Position

	return ml.Stream[start:end]

}

func (ml *MetaLexer) Lex() ([]Token, error) {

	var tokens []Token

	for !ml.aatEnd() {

		if ml.Current == '(' || ml.Current == ')' {

			token, err := consumeToken
			if err != nil {
				goto ignore
			}

			append(tokens, ConsumeToken())
		}

	ignore:
		//
		ml.advance()
	}

	return nil, nil
}
