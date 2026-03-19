// =============================================================================
// xmlgen — generates Go structs from XSD schema files
// author: abdellah el morabit
// =============================================================================

package meta

import (
	"fmt"
	"os"
	"path/filepath"
)

type TokenType byte

const (
	TOKEN_UNDEFINED TokenType = iota
	TOKEN_SCHEMA
	TOKEN_ELEMENT
	TOKEN_COMPLEX_TYPE
	TOKEN_SIMPLE_TYPE
	TOKEN_SEQUENCE
	TOKEN_RESTRICTION
	TOKEN_ENUMERATION
	TOKEN_ATTR_NAME
	TOKEN_ATTR_TYPE
	TOKEN_ATTR_BASE
	TOKEN_ATTR_VALUE
	TOKEN_ATTR_ELEMENT_FORM_DEFAULT
	TOKEN_ATTR_XMLNS
	TOKEN_TYPE_STRING
	TOKEN_TYPE_DATETIME
	TOKEN_TYPE_INT
	TOKEN_TYPE_BOOLEAN
	TOKEN_TYPE_DECIMAL
	TOKEN_IDENT
	TOKEN_STRING_LIT

	TOKEN_COLON    TokenType = ':'
	TOKEN_EQUALS   TokenType = '='
	TOKEN_SLASH    TokenType = '/'
	TOKEN_LANGLE   TokenType = '<'
	TOKEN_RANGLE   TokenType = '>'
	TOKEN_QUESTION TokenType = '?'
)

type Node struct {
	Lexeme []byte
	Next   *Node
	First  *Node
	Last   *Node
}

type Tree struct {
	Root    *Node
	Current *Node
}

type Token struct {
	Type   TokenType
	Lexeme []byte
	Line   int
	Column int
}

type MetaLexer struct {
	FileName string
	Stream   []byte
	Start    int
	Current  int
	Line     int
	Column   int
}

type MetaError struct {
	FileName string
	Content  string
	Line     int
	Column   int
}

func (e *MetaError) Error() string {
	return fmt.Sprintf("error in %s at line %d, column %d: %s", e.FileName, e.Line, e.Column)
}

func IsXsd(fileName string) bool {
	return filepath.Ext(fileName) == ".xsd"
}

func (*MetaLexer) LoadFile(path string, file string) ([]byte, error) {

	content, err := os.ReadFile(filepath.Join(path, file))

	if err != nil {
		return nil, &MetaError{
			FileName: file,
			Content:  fmt.Sprintf("failed to open file >>> %v <<<", err),
		}
	}

	return content, nil

}

func isWhiteSpace(character byte) bool {

	if character == '\n' || character == '\r' || character == '\t' || character == ' ' {
		return true
	}
	return false
}

func Lex(content []byte) []Token {
	tokens := []Token{}

	for i := 0; i < len(content); i++ {

		ch := content[i]

		if isWhiteSpace(ch) {
			continue
		}

		switch TokenType(ch) {

		case TOKEN_LANGLE:
			{
				if TokenType(content[i+1]) == TOKEN_QUESTION {

					continue
				}
			}
		case TOKEN_RANGLE:
		case TOKEN_COLON:
		case TOKEN_EQUALS:
		case TOKEN_SLASH:
			{
				if TokenType(content[i+1]) == TOKEN_RANGLE {

					// TODO(nasr): end of a lexeme
				}

			}
		case TOKEN_QUESTION:

		}

		// TODO(nasr): match and append tokens
	}

	return tokens
}
