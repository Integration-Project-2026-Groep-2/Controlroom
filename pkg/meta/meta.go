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

// NOTE(nasr): AI genereated Xsd to go mapping
var XsdToGo = map[string]string{
	"xs:string":          "string",
	"xs:boolean":         "bool",
	"xs:int":             "int",
	"xs:integer":         "int",
	"xs:long":            "int64",
	"xs:short":           "int16",
	"xs:byte":            "int8",
	"xs:float":           "float32",
	"xs:double":          "float64",
	"xs:decimal":         "float64",
	"xs:dateTime":        "time.Time",
	"xs:date":            "time.Time",
	"xs:time":            "time.Time",
	"xs:duration":        "time.Duration",
	"xs:base64Binary":    "[]byte",
	"xs:hexBinary":       "[]byte",
	"xs:anyURI":          "string",
	"xs:positiveInteger": "uint",
}

type SequenceType string

const (
	SEQUENCE_COMPLEX_TYPE SequenceType = "complexType"
	SEQUENCE_SIMPLE_TYPE               = "simpleType"
	SEQUENCE_SCHEMA_TYPE               = "schema"
)

type Sequence struct {
	SType      SequenceType
	Attributes string
	Type       string
}

type Node struct {
	Lexeme []byte
	Next   *Node
	First  *Node
	Last   *Node
}

type AST struct {
	Root    *Node
	Current *Node
}

type Token struct {
	Type   TokenType
	Lexeme []byte
}

type MetaLexer struct {
	FileName string
	Stream   []byte
	Start    int
	Current  int
}

type MetaError struct {
	FileName string
	Content  string
}

func (e *MetaError) Error() string {
	return fmt.Sprintf("error in %s: %s", e.FileName, e.Content)
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

		ch := TokenType(content[i])

		switch ch {

		case TOKEN_LANGLE:
			{
				i++
				next := TokenType(content[i+1])
				newLexeme := []byte{}

				if next == TOKEN_QUESTION {
					continue
				} else {

					// continue checking the line while the final thing isnt a slash or a rangle
					for next != TOKEN_RANGLE && next != TOKEN_SLASH {

						// break before reaching out of bounds
						// TODO(nasr): write a general check that checks for EOF
						if i+1 >= len(content) {
							break
						}

						newLexeme = append(newLexeme, content[i])
						i++
						next = TokenType(content[i])
					}

					newToken := Token{
						Type:   TOKEN_UNDEFINED,
						Lexeme: newLexeme,
					}
					tokens = append(tokens, newToken)
				}
			}
		case TOKEN_RANGLE:
		case TOKEN_SLASH:
			{
				if TokenType(content[i]) == TOKEN_RANGLE {

					// TODO(nasr): end of a lexeme
				}

			}
		case TOKEN_QUESTION:
			{
				continue

			}

		case TOKEN_COLON:
		case TOKEN_EQUALS:

		}

	}

	return tokens
}
