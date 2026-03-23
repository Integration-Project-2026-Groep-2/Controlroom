// =============================================================================
// xmlgen — generates Go structs from XSD schema files
// author: abdellah el morabit
// =============================================================================

package meta

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// TagType represents an XSD element tag name as a string identifier.
type TagType string

// NOTE(nasr): AI generated list of xsd tags
// XSD element tag constants used to identify structural components of a schema.
const (
	TAG_SCHEMA          TagType = "schema"
	TAG_ELEMENT         TagType = "element"
	TAG_COMPLEX_TYPE    TagType = "complexType"
	TAG_SIMPLE_TYPE     TagType = "simpleType"
	TAG_SEQUENCE        TagType = "sequence"
	TAG_RESTRICTION     TagType = "restriction"
	TAG_ENUMERATION     TagType = "enumeration"
	TAG_SIMPLE_CONTENT  TagType = "simpleContent"
	TAG_COMPLEX_CONTENT TagType = "complexContent"
	TAG_EXTENSION       TagType = "extension"
	TAG_ATTRIBUTE       TagType = "attribute"
	TAG_CHOICE          TagType = "choice"
	TAG_ALL             TagType = "all"
)

// TokenType represents a single lexical token class produced by the lexer.
// ASCII punctuation characters are stored directly as their byte values.
type TokenType byte

// Token constants for XSD opening tags, closing tags, attribute names,
// primitive type keywords, and punctuation delimiters.
const (
	// TOKEN_UNDEFINED_EOF marks an unclassified token or end of stream.
	TOKEN_UNDEFINED_EOF TokenType = iota

	// Opening tag tokens — emitted when the lexer encounters <xs:TAG.
	TOKEN_SCHEMA_OPENING
	TOKEN_ELEMENT_OPENING
	TOKEN_COMPLEX_TYPE_OPENING
	TOKEN_SIMPLE_TYPE_OPENING
	TOKEN_SEQUENCE_OPENING
	TOKEN_RESTRICTION_OPENING
	TOKEN_ENUMERATION_OPENING
	TOKEN_SIMPLE_CONTENT_OPENING
	TOKEN_COMPLEX_CONTENT_OPENING
	TOKEN_EXTENSION_OPENING
	TOKEN_ATTRIBUTE_OPENING
	TOKEN_CHOICE_OPENING
	TOKEN_ALL_OPENING

	// Closing tag tokens — emitted when the lexer encounters </xs:TAG>.
	TOKEN_SCHEMA_CLOSING
	TOKEN_ELEMENT_CLOSING
	TOKEN_COMPLEX_TYPE_CLOSING
	TOKEN_SIMPLE_TYPE_CLOSING
	TOKEN_SEQUENCE_CLOSING
	TOKEN_RESTRICTION_CLOSING
	TOKEN_ENUMERATION_CLOSING
	TOKEN_SIMPLE_CONTENT_CLOSING
	TOKEN_COMPLEX_CONTENT_CLOSING
	TOKEN_EXTENSION_CLOSING
	TOKEN_ATTRIBUTE_CLOSING
	TOKEN_CHOICE_CLOSING
	TOKEN_ALL_CLOSING

	// Attribute keyword tokens — emitted when the lexer encounters a known
	// XSD attribute name inside a tag.
	TOKEN_ATTR_NAME
	TOKEN_ATTR_TYPE
	TOKEN_ATTR_BASE
	TOKEN_ATTR_VALUE
	TOKEN_ATTR_FIXED
	TOKEN_ATTR_ELEMENT_FORM_DEFAULT
	TOKEN_ATTR_XMLNS

	// Primitive XSD type keyword tokens.
	TOKEN_TYPE_STRING
	TOKEN_TYPE_DATETIME
	TOKEN_TYPE_INT
	TOKEN_TYPE_BOOLEAN
	TOKEN_TYPE_DECIMAL

	// TOKEN_IDENT is a generic identifier lexeme (tag or attribute names not
	// matched by any specific keyword token).
	TOKEN_IDENT
	// TOKEN_STRING_LIT is a quoted string literal value (attribute value content).
	TOKEN_STRING_LIT

	// Punctuation delimiters stored as their ASCII byte values.
	TOKEN_COLON    TokenType = ':'
	TOKEN_EQUALS   TokenType = '='
	TOKEN_SLASH    TokenType = '/'
	TOKEN_LANGLE   TokenType = '<'
	TOKEN_RANGLE   TokenType = '>'
	TOKEN_QUESTION TokenType = '?'
	TOKEN_QUOTE    TokenType = '"'
)

// NOTE(nasr): AI generated XSD to Go mapping
// XsdToGo maps XSD primitive type names to their equivalent Go type strings.
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

// Sequence represents a parsed XSD sequence compositor with its tag type
// and raw attribute string.
type Sequence struct {
	Type TagType
	Attr string
}

// Node is a single node in the AST. Lexeme holds the raw bytes for this node.
// Next points to the next sibling, First to the first child, Last to the last child.
type Node struct {
	Lexeme []byte
	Next   *Node
	Parent *Node
	First  *Node
	Last   *Node
}

// AST holds the root of the parsed syntax tree and a cursor to the node
// currently being built during parsing.
type AST struct {
	Root    *Node
	Current *Node
}

// Token is a single lexical unit produced by MetaLexer.
// Type classifies the token; Lexeme holds the raw bytes from the source.
type Token struct {
	Type   TokenType
	Lexeme []byte
	Tag    TagType
}

// MetaLexer holds the raw XSD byte stream, the token list produced by Lex,
// and the current read position within the stream.
type MetaLexer struct {
	Stream   []byte
	Tokens   []Token
	Position int
}

// MetaError is a structured error that records the file name and a description
// of what went wrong during lexing or file loading.
type MetaError struct {
	FileName string
	Content  string
}

// Error implements the error interface, returning a formatted message that
// includes the file name and error description.
func (e *MetaError) Error() string {
	return fmt.Sprintf("error in %s: %s", e.FileName, e.Content)
}

// IsXsd reports whether fileName has the ".xsd" extension.
func IsXsd(fileName string) bool {
	return filepath.Ext(fileName) == ".xsd"
}

// LoadFile reads the XSD file at path/file into the lexer stream and resets
// the read position to zero. Returns a MetaError if the file cannot be read.
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

// current returns the byte at the current stream position.
func (ml *MetaLexer) current() byte {
	return ml.Stream[ml.Position]
}

// peek returns the byte one position ahead of the current position,
// or 0 if the end of the stream has been reached.
func (ml *MetaLexer) peek() byte {
	if ml.Position+1 < len(ml.Stream) {
		return ml.Stream[ml.Position+1]
	}
	return 0
}

// advance moves the read position forward by one byte,
// provided the current byte is not a null terminator.
func (ml *MetaLexer) advance() {
	if ml.Stream[ml.Position] != 0 {
		ml.Position++
	}
}

// atEnd reports whether the read position has reached or passed the end of the stream.
func (ml *MetaLexer) atEnd() bool {
	return ml.Position >= len(ml.Stream)
}

// isDelimiter reports whether the byte at the current position is an XSD
// delimiter: one of < > / = ? : " or any whitespace character.
func (ml *MetaLexer) isDelimiter() bool {
	c := ml.current()
	return c == '<' || c == '>' || c == '/' || c == '=' ||
		c == '?' || c == ':' || c == '"' || isWhiteSpace(c)
}

// isWhiteSpace reports whether c is a space, tab, newline, or carriage return.
func isWhiteSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// skipWhiteSpace advances past all consecutive whitespace characters.
func (ml *MetaLexer) skipWhiteSpace() {
	for !ml.atEnd() && isWhiteSpace(ml.current()) {
		ml.advance()
	}
}

// getByteAtPosition returns the byte at Position + offset within the stream.
// TODO(nasr): add checking for out of bounds errors
func (ml *MetaLexer) getByteAtPosition(offset int) byte {
	return ml.Stream[ml.Position]
}

// checkTag inspects the stream at the current position and returns the TagType
// of the XSD element tag found there.
func (ml *MetaLexer) checkTag() TagType {
	index := 0
	for ml.Position+index < len(ml.Stream) && !isWhiteSpace(ml.Stream[ml.Position+index]) {
		index++
	}
	tag := ml.Stream[ml.Position : ml.Position+index]
	return TagType(tag)
}

// PushNextNode appends node as the next sibling of the current AST node.
func (ast *AST) PushNextNode(node *Node) error {
	if node == nil {
		// TODO(nasr): Logging
		return nil
	}

	if ast.Root == nil {
		ast.Root = node
	}

	node.Parent = ast.Current

	ast.Current.Next = node
	ast.Current = node

	return nil
}

// PushChildNode appends node as a child of the current AST node.
func (ast *AST) PushChildNode(node *Node) error {

	if node == nil || ast.Root == nil {
		// TODO(nasr): Logging
		return nil
	}

	ast.Current.First = node
	ast.Current.Last = node
	ast.Current.Parent = ast.Current

	ast.Current = node

	return nil
}

// Lex scans the full stream and populates ml.Tokens. The token slice is reset
// before scanning begins. Processing instructions (<?xml ... ?>) are skipped.
// Namespace prefixes (xs:) are stripped from tag lexemes. Returns nil on success.
func (ml *MetaLexer) Lex(ast *AST) error {

	ml.Tokens = ml.Tokens[:0]
	tokensIndex := 0

	for !ml.atEnd() {
		ml.skipWhiteSpace()
		if ml.atEnd() {
			break
		}

		c := TokenType(ml.current())

		switch c {

		case TOKEN_LANGLE:
			ml.advance()
			if ml.atEnd() {
				break
			}

			// skip <?xml ... ?> processing instructions
			if TokenType(ml.current()) == TOKEN_QUESTION {
				for !ml.atEnd() && TokenType(ml.current()) != TOKEN_RANGLE {
					ml.advance()
				}
				// consume '>'
				ml.advance()
				continue
			}

			// collect the tag content into a lexeme
			lexeme := []byte{}

			for !ml.atEnd() {
				cur := TokenType(ml.current())
				if cur == TOKEN_RANGLE || cur == TOKEN_SLASH {
					break
				}

				// skip namespace prefix (everything up to and including ':')
				if cur == TOKEN_COLON {
					// TODO(nasr): check if it's a closing or opening tag
					lexeme = lexeme[2:]         
					ml.advance()
					continue
				}

				if !isWhiteSpace(ml.current()) {
					lexeme = append(lexeme, ml.current())
				}
				ml.advance()
			}

			if len(lexeme) > 0 {
				token := Token{
					Type:   TOKEN_UNDEFINED_EOF,
					Lexeme: lexeme,
					Tag:    TagType(lexeme),
				}

				{
					ml.Tokens = append(ml.Tokens, token)
					tokensIndex++
				}

				node := &Node{Lexeme: lexeme}

				// first node becomes root, subsequent nodes become children
				if ast.Root == nil {
					ast.Root = node
					ast.Current = node
				} else {
					ast.PushChildNode(node)
					log.Printf(string(lexeme))
				}
			}

		// open tag is fully closed nothing extra to do, node already pushed above
		case TOKEN_RANGLE:
			ml.advance()

		case TOKEN_SLASH:

			ml.advance()

			// closing tag: walk back up to parent before skipping to '>'
			if ast.Current != nil && ast.Current.Parent != nil {
				ast.Current = ast.Current.Parent
			}

			for !ml.atEnd() && TokenType(ml.current()) != TOKEN_RANGLE {
				ml.advance()
			}

			// consume '>'
			ml.advance()

		case TOKEN_EQUALS:

            var lexeme []byte
			node := &Node{Lexeme: lexeme}
		    ast.PushChildNode(node)

			ml.advance()

		default:
			ml.advance()
		}
	}

	return nil
}

func WriteGoStruct(ast *AST, folderPath string) error {

	node := AST.Root

	for node := AST.Current 
	return nil
}
