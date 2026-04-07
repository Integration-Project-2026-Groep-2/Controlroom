// =============================================================================
// xmlgen — generates Go structs from XSD schema files
// author: abdellah el morabit
// =============================================================================

package meta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// TagType represents an XSD element tag name as a string identifier.
type TagType string

// NOTE(nasr): AI generated list of xsd tags
const (
	tagSchema         TagType = "schema"
	tagElement        TagType = "element"
	tagComplexType    TagType = "complexType"
	tagSimpleType     TagType = "simpleType"
	tagSequence       TagType = "sequence"
	tagRestriction    TagType = "restriction"
	tagEnumeration    TagType = "enumeration"
	tagSimpleContent  TagType = "simpleContent"
	tagComplexContent TagType = "complexContent"
	tagExtension      TagType = "extension"
	tagAttribute      TagType = "attribute"
	tagChoice         TagType = "choice"
	tagAll            TagType = "all"
)

// tokenType represents a single lexical token class produced by the lexer.
type tokenType byte

const (
	tokenUndefinedEof tokenType = iota
)

// xsdToGo NOTE(nasr): AI generated XSD to Go mapping
// TODO(nasr): don't forget when types are not found just add them to the mapping
var xsdToGo = map[string]string{
	"xs:string":             "string",
	"xs:boolean":            "bool",
	"xs:int":                "int",
	"xs:integer":            "int",
	"xs:long":               "int64",
	"xs:short":              "int16",
	"xs:byte":               "int8",
	"xs:float":              "float32",
	"xs:double":             "float64",
	"xs:decimal":            "float64",
	"xs:dateTime":           "time.Time",
	"xs:date":               "time.Time",
	"xs:time":               "time.Time",
	"xs:duration":           "time.Duration",
	"xs:base64Binary":       "[]byte",
	"xs:hexBinary":          "[]byte",
	"xs:anyURI":             "string",
	"xs:positiveInteger":    "uint",
	"xs:nonNegativeInteger": "uint",
}

// attrs holds the parsed XML attributes of a single element node.
type attrs struct {
	Name   string
	Type   string
	Base   string
	Value  string
	Fixed  string
	MinOcc string
	MaxOcc string
	Use    string
}

// node is a single node in the AST.
// Tag is the XSD tag type. attrs holds all parsed XML attributes.
// Next is the next sibling, First/Last are the first/last child, Parent is the enclosing node.
type node struct {
	Tag    TagType
	Attrs  attrs
	Next   *node
	Parent *node
	First  *node
	Last   *node
}

// AST holds the root of the parsed syntax tree and a cursor used during parsing.
// Abstract syntax tree
type AST struct {
	Root    *node
	Current *node
}

// token is a single lexical unit produced by MetaLexer.
type token struct {
	Type   tokenType
	Lexeme []byte
	Tag    TagType
}

// Lexer MetaLexer holds the raw XSD byte stream, the token list, and the read position.
type Lexer struct {
	Stream   []byte
	Tokens   []token
	Position int
}

// metaError is a structured parse/IO error.
type metaError struct {
	FileName string
	Content  string
}

func (e *metaError) Error() string {
	return fmt.Sprintf("error in %s: %s", e.FileName, e.Content)
}

// IsXsd reports whether fileName has the ".xsd" extension.
func IsXsd(fileName string) bool {
	return filepath.Ext(fileName) == ".xsd"
}

// LoadFile reads the XSD file at path/file into the lexer stream.
func (ml *Lexer) LoadFile(path string, file string) error {
	content, err := os.ReadFile(filepath.Join(path, file))
	if err != nil {
		return &metaError{
			FileName: file,
			Content:  fmt.Sprintf("failed to open file >>> %v <<<", err),
		}
	}
	ml.Stream = content
	ml.Position = 0
	return nil
}

// =============================================================================
// Lexer helper functions
// =============================================================================

func (ml *Lexer) current() byte {
	return ml.Stream[ml.Position]
}

func (ml *Lexer) atEnd() bool {
	return ml.Position >= len(ml.Stream)
}

func (ml *Lexer) advance() {
	if !ml.atEnd() {
		ml.Position++
	}
}

func (ml *Lexer) peek() byte {
	if ml.Position+1 < len(ml.Stream) {
		return ml.Stream[ml.Position+1]
	}
	return 0
}

// =============================================================================

func isWhiteSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func (ml *Lexer) skipWhiteSpace() {
	for !ml.atEnd() && isWhiteSpace(ml.current()) {
		ml.advance()
	}
}

// isAttrDelim returns true for bytes that end an attribute name token.
func isAttrDelim(c byte) bool {
	return c == '=' || c == '>' || c == '/' || c == '"' || isWhiteSpace(c)
}

// isTagDelim returns true for bytes that end a tag name token.
func isTagDelim(c byte) bool {
	return c == '>' || c == '/' || isWhiteSpace(c)
}

// readUntil reads bytes up to (but not including) any byte where stop returns true.
func (ml *Lexer) readUntil(stop func(byte) bool) []byte {
	start := ml.Position
	for !ml.atEnd() && !stop(ml.current()) {
		ml.advance()
	}
	return ml.Stream[start:ml.Position]
}

// readQuotedString reads past the opening '"', collects until closing '"', and
// advances past it. Returns the content between the quotes.
func (ml *Lexer) readQuotedString() []byte {
	ml.advance() // skip opening '"'
	start := ml.Position
	for !ml.atEnd() && ml.current() != '"' {
		ml.advance()
	}
	val := ml.Stream[start:ml.Position]
	if !ml.atEnd() {
		ml.advance()
		// skip closing '"'
	}
	return val
}

// stripNamespace removes everything up to and including the first ':'.
func stripNamespace(raw []byte) []byte {
	for i, b := range raw {
		if b == ':' {
			return raw[i+1:]
		}
	}
	return raw
}

// parseAttrs reads attribute key="value" pairs until '>' or '/'.
// The position is left ON the terminating '>' or '/'.
func (ml *Lexer) parseAttrs() attrs {
	var a attrs
	for !ml.atEnd() {
		ml.skipWhiteSpace()
		if ml.atEnd() {
			break
		}
		c := ml.current()
		if c == '>' || c == '/' {
			break
		}

		// read attribute name (may contain namespace prefix e.g. xmlns:xs)
		rawKey := ml.readUntil(isAttrDelim)
		if len(rawKey) == 0 {
			ml.advance() // skip stray delimiter
			continue
		}
		localKey := string(stripNamespace(rawKey))

		// NOTE(nasr): skipping the attribute if it's empty
		ml.skipWhiteSpace()
		if ml.atEnd() || ml.current() != '=' {
			continue
		}

		ml.advance() // consume '='

		ml.skipWhiteSpace()
		if ml.atEnd() {
			break
		}

		var val []byte
		if ml.current() == '"' {
			val = ml.readQuotedString()
		} else {
			val = ml.readUntil(isAttrDelim)
		}

		switch localKey {
		case "name":
			a.Name = string(val)
		case "type":
			a.Type = string(val)
		case "base":
			a.Base = string(val)
		case "value":
			a.Value = string(val)
		case "fixed":
			a.Fixed = string(val)
		case "minOccurs":
			a.MinOcc = string(val)
		case "maxOccurs":
			a.MaxOcc = string(val)
		case "use":
			a.Use = string(val)
		}
	}
	return a
}

// knownTag returns true when the local name is a recognised XSD structural tag.
func knownTag(local string) bool {
	switch TagType(local) {
	case tagSchema, tagElement, tagComplexType, tagSimpleType,
		tagSequence, tagRestriction, tagEnumeration, tagSimpleContent,
		tagComplexContent, tagExtension, tagAttribute, tagChoice, tagAll:
		return true
	}
	return false
}

// pushChild appends node as a child of the current node and descends into it.
func (ast *AST) pushChild(node *node) {
	if node == nil {
		return
	}
	node.Parent = ast.Current // correct: parent is the CURRENT node, not node itself
	if ast.Current.First == nil {
		ast.Current.First = node
		ast.Current.Last = node
	} else {
		ast.Current.Last.Next = node
		ast.Current.Last = node
	}
	ast.Current = node // descend
}

// pushSibling adds node as a sibling under the same parent without descending. Used for self-closing tags.
func (ast *AST) pushSibling(node *node) {
	if node == nil || ast.Current == nil {
		return
	}
	parent := ast.Current.Parent
	node.Parent = parent
	if parent != nil {
		if parent.Last == nil {
			parent.First = node
			parent.Last = node
		} else {
			parent.Last.Next = node
			parent.Last = node
		}
	}
}

// popToParent moves the cursor one level up toward the root.
func (ast *AST) popToParent() {
	if ast.Current != nil && ast.Current.Parent != nil {
		ast.Current = ast.Current.Parent
	}
}

// Lex scans the full stream and builds the AST.
func (ml *Lexer) Lex(ast *AST) error {
	ml.Tokens = ml.Tokens[:0]

	for !ml.atEnd() {
		ml.skipWhiteSpace()
		if ml.atEnd() {
			break
		}

		// skip non-tag text content
		if ml.current() != '<' {
			ml.advance()
			continue
		}

		// consume '<'
		ml.advance()
		if ml.atEnd() {
			break
		}

		// processing instruction <?...?>
		if ml.current() == '?' {
			for !ml.atEnd() && ml.current() != '>' {
				ml.advance()
			}
			ml.advance()
			continue
		}

		// NOTE(nasr): skip comments by checking for lines or what are they called -> - <-
		if ml.current() == '!' {
			ml.advance()
			for !ml.atEnd() {
				if ml.current() == '-' && ml.Position+2 < len(ml.Stream) &&
					ml.Stream[ml.Position+1] == '-' && ml.Stream[ml.Position+2] == '>' {
					ml.Position += 3
					break
				}
				ml.advance()
			}
			continue
		}

		// closing tag </xs:tag>
		isClosing := false
		if ml.current() == '/' {
			isClosing = true
			ml.advance()
		}

		// read full raw tag
		rawTag := ml.readUntil(isTagDelim)
		local := string(stripNamespace(rawTag))

		if !knownTag(local) {
			// skip unknown tags wholesale
			for !ml.atEnd() && ml.current() != '>' {
				ml.advance()
			}
			ml.advance()
			continue
		}

		tag := TagType(local)

		if isClosing {
			ast.popToParent()
			for !ml.atEnd() && ml.current() != '>' {
				ml.advance()
			}
			ml.advance()
			continue
		}

		// opening or self-closing: parse attributes
		ml.skipWhiteSpace()
		attrs := ml.parseAttrs()

		// detect self-close: position should now be on '/' or '>'
		ml.skipWhiteSpace()
		selfClose := false
		if !ml.atEnd() && ml.current() == '/' {
			selfClose = true
			ml.advance()
		}
		if !ml.atEnd() && ml.current() == '>' {
			ml.advance()
		}

		node := &node{Tag: tag, Attrs: attrs}

		ml.Tokens = append(ml.Tokens, token{
			Type:   tokenUndefinedEof,
			Lexeme: []byte(local),
			Tag:    tag,
		})

		if ast.Root == nil {
			ast.Root = node
			ast.Current = node
			continue
		}

		// Always pushChild — self-closing tags are children of the current node
		// just like opening tags. The only difference is we immediately pop back
		// so the cursor does not descend into them (they have no children).
		ast.pushChild(node)
		if selfClose {
			ast.popToParent()
		}
	}

	return nil
}

// goIdent converts an XSD name to a capitalized Go exported identifier.
func goIdent(name string) string {
	if name == "" {
		return "Unknown"
	}
	var b strings.Builder
	capNext := true
	for _, r := range name {
		if r == '-' || r == '_' || r == '.' {
			capNext = true
			continue
		}
		if capNext {
			b.WriteRune(unicode.ToUpper(r))
			capNext = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// resolveGoType converts an XSD type attribute value to a Go type string.
// Primitive types come from xsdToGo; unknown types become struct references.
func resolveGoType(xsdType string) string {
	if gt, ok := xsdToGo[xsdType]; ok {
		return gt
	}
	local := xsdType
	if _, after, ok := strings.Cut(xsdType, ":"); ok {
		local = after
	}
	return goIdent(local)
}

// jsonKey converts a camelCase or PascalCase XSD name to snake_case for JSON tags.
// e.g. "serviceId" -> "service_id", "Timestamp" -> "timestamp"
func jsonKey(name string) string {
	var out []rune
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(runes[i-1]) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(r))
	}
	return string(out)
}

// isRequired reports whether an XSD element or attribute is required.
// Elements are required when minOccurs is absent (defaults to 1) or explicitly "1".
// Attributes are required when use="required".
func isRequired(attrs attrs, isAttr bool) bool {
	if isAttr {
		return attrs.Use == "required"
	}
	return attrs.MinOcc == "" || attrs.MinOcc == "1"
}

// structField holds data for one Go struct field.
type structField struct {
	GoName   string
	GoType   string
	XMLTag   string
	IsAttr   bool
	JSONTag  string
	Validate string
}

// buildTag composes the full struct tag string from a structField.
func buildTag(f structField) string {
	var xmlPart, jsonPart, validatePart string

	switch {
	case f.XMLTag == ",chardata":
		xmlPart = `xml:",chardata"`
	case f.IsAttr:
		xmlPart = fmt.Sprintf(`xml:"%s,attr"`, f.XMLTag)
	default:
		xmlPart = fmt.Sprintf(`xml:"%s"`, f.XMLTag)
	}

	parts := []string{xmlPart}

	if f.JSONTag != "" {
		jsonPart = fmt.Sprintf(`json:"%s"`, f.JSONTag)
		parts = append(parts, jsonPart)
	}

	if f.Validate != "" {
		validatePart = fmt.Sprintf(`validate:"%s"`, f.Validate)
		parts = append(parts, validatePart)
	}

	return "`" + strings.Join(parts, " ") + "`"
}

// collectFields walks direct children of a node and returns fields for the
// generated struct. Transparent compositor nodes (sequence / choice / all)
// are recursed through transparently.
func collectFields(node *node) []structField {
	var fields []structField
	for child := node.First; child != nil; child = child.Next {
		switch child.Tag {
		case tagSequence, tagChoice, tagAll:
			fields = append(fields, collectFields(child)...)

		case tagElement:
			name := child.Attrs.Name
			if name == "" {
				continue
			}
			goType := "string"
			if child.Attrs.Type != "" {
				goType = resolveGoType(child.Attrs.Type)
			} else {
				// inline anonymous complexType child?
				for inner := child.First; inner != nil; inner = inner.Next {
					if inner.Tag == tagComplexType {
						goType = goIdent(name) + "Type"
						break
					}
				}
			}
			isSlice := child.Attrs.MaxOcc == "unbounded" ||
				(child.Attrs.MaxOcc != "" && child.Attrs.MaxOcc != "1" && child.Attrs.MaxOcc != "0")
			if isSlice {
				goType = "[]" + goType
			}
			validate := ""
			if isRequired(child.Attrs, false) {
				validate = "required"
			}
			fields = append(fields, structField{
				GoName:   goIdent(name),
				GoType:   goType,
				XMLTag:   name,
				IsAttr:   false,
				JSONTag:  jsonKey(name),
				Validate: validate,
			})

		case tagAttribute:
			name := child.Attrs.Name
			if name == "" {
				continue
			}
			goType := "string"
			if child.Attrs.Type != "" {
				goType = resolveGoType(child.Attrs.Type)
			}
			validate := ""
			if isRequired(child.Attrs, true) {
				validate = "required"
			}
			fields = append(fields, structField{
				GoName:   goIdent(name),
				GoType:   goType,
				XMLTag:   name,
				IsAttr:   true,
				JSONTag:  jsonKey(name),
				Validate: validate,
			})
		}
	}
	return fields
}

// collectEnumValues returns the xs:enumeration value strings under a
// simpleType > restriction node.
func collectEnumValues(node *node) []string {
	var vals []string
	for child := node.First; child != nil; child = child.Next {
		if child.Tag == tagRestriction {
			for e := child.First; e != nil; e = e.Next {
				if e.Tag == tagEnumeration && e.Attrs.Value != "" {
					vals = append(vals, e.Attrs.Value)
				}
			}
		}
	}
	return vals
}

// collectSimpleContentFields returns the fields for a complexType whose body
// is a simpleContent > extension node. The extension base becomes a chardata
// Value field; any xs:attribute children become attribute fields.
func collectSimpleContentFields(node *node) (fields []structField, ok bool) {
	for child := node.First; child != nil; child = child.Next {
		if child.Tag != tagSimpleContent {
			continue
		}
		for ext := child.First; ext != nil; ext = ext.Next {
			if ext.Tag != tagExtension {
				continue
			}
			goType := "string"
			if ext.Attrs.Base != "" {
				goType = resolveGoType(ext.Attrs.Base)
			}
			// chardata field for the element text value — no json/validate on the raw Value field
			fields = append(fields, structField{
				GoName:   "Value",
				GoType:   goType,
				XMLTag:   ",chardata",
				IsAttr:   false,
				JSONTag:  "value",
				Validate: "",
			})
			// attributes on the extension
			for attr := ext.First; attr != nil; attr = ext.Next {
				if attr.Tag != tagAttribute || attr.Attrs.Name == "" {
					continue
				}
				attrType := "string"
				if attr.Attrs.Type != "" {
					attrType = resolveGoType(attr.Attrs.Type)
				}
				validate := ""
				if isRequired(attr.Attrs, true) {
					validate = "required"
				}
				fields = append(fields, structField{
					GoName:   goIdent(attr.Attrs.Name),
					GoType:   attrType,
					XMLTag:   attr.Attrs.Name,
					IsAttr:   true,
					JSONTag:  jsonKey(attr.Attrs.Name),
					Validate: validate,
				})
			}
			return fields, true
		}
	}
	return nil, false
}

// hasSimpleContent reports whether a complexType node uses simpleContent.
func hasSimpleContent(node *node) bool {
	for child := node.First; child != nil; child = child.Next {
		if child.Tag == tagSimpleContent {
			return true
		}
	}
	return false
}

// buildStructs walks the AST depth-first and emits Go type declarations.
// seen prevents duplicate declarations across sibling nodes.
func buildStructs(node *node, buf *strings.Builder, seen map[string]bool) {
	if node == nil {
		return
	}

	switch node.Tag {

	// complexType struct (regular or simpleContent flavour)
	case tagComplexType:
		name := node.Attrs.Name
		if name == "" && node.Parent != nil && node.Parent.Tag == tagElement {
			name = node.Parent.Attrs.Name
		}
		if name != "" {
			ident := goIdent(name)
			if !seen[ident] {
				seen[ident] = true
				if hasSimpleContent(node) {
					fields, _ := collectSimpleContentFields(node)
					writeStruct(buf, ident, fields)
				} else {
					writeStruct(buf, ident, collectFields(node))
				}
			}
		}
		for child := node.First; child != nil; child = child.Next {
			buildStructs(child, buf, seen)
		}

	// simpleType string typedef + typed const block for enumerations
	case tagSimpleType:
		name := node.Attrs.Name
		if name == "" {
			break
		}
		ident := goIdent(name)
		if seen[ident] {
			break
		}
		seen[ident] = true

		vals := collectEnumValues(node)
		if len(vals) == 0 {
			// plain restriction with no enumeration — emit a type alias only
			_, _ = fmt.Fprintf(buf, "type %s string\n\n", ident)
			break
		}
		// string-backed type + const block
		_, _ = fmt.Fprintf(buf, "type %s string\n\n", ident)
		_, _ = fmt.Fprintf(buf, "const (\n")
		for _, v := range vals {
			constName := ident + goIdent(v)
			_, _ = fmt.Fprintf(buf, "\t%-32s %s = %q\n", constName, ident, v)
		}
		_, _ = fmt.Fprintf(buf, ")\n\n")

	default:
		for child := node.First; child != nil; child = child.Next {
			buildStructs(child, buf, seen)
		}
	}

	buildStructs(node.Next, buf, seen)
}

// NOTE(nasr): ai generated struct generation :)
// =============================================================================
// writeStruct emits a single Go struct definition.
func writeStruct(buf *strings.Builder, name string, fields []structField) {
	buf.WriteString("type ")
	buf.WriteString(name)
	buf.WriteString(" struct {\n")

	xmlElem := name
	_, _ = fmt.Fprintf(buf, "\tXMLName xml.Name `xml:\"%s\" json:\"%s\"`\n", xmlElem, xmlElem)

	for _, f := range fields {
		tag := buildTag(f)
		_, _ = fmt.Fprintf(buf, "\t%-24s %-20s %s\n", f.GoName, f.GoType, tag)
	}
	buf.WriteString("}\n\n")
}

// xsdToGoFileName converts an XSD filename (e.g. "user-type.xsd") to a Go
// source filename (e.g. "user_type.go"). Hyphens become underscores because
// Go tooling does not allow hyphens in source file names.
func xsdToGoFileName(xsdFile string) string {
	base := strings.TrimSuffix(filepath.Base(xsdFile), filepath.Ext(xsdFile))
	base = strings.ReplaceAll(base, "-", "_")
	return base + ".go"
}

// WriteGoStruct walks the AST and writes a self-contained Go source file into
// folderPath. The output filename is derived from xsdFile so that each XSD
// produces its own .go file and no two XSDs can overwrite each other.
// Package is set to "gen" to match the conventional generated-code sub-package.
func WriteGoStruct(ast *AST, folderPath string, xsdFile string) error {
	if ast == nil || ast.Root == nil {
		return &metaError{FileName: xsdFile, Content: "AST is nil or empty"}
	}

	var buf strings.Builder
	buf.WriteString("// Code generated by xmlgen. DO NOT EDIT.\n")
	buf.WriteString("// Author: Abdellah El Morabit.\n")
	buf.WriteString("package gen\n\n")
	buf.WriteString("import (\n\t\"encoding/xml\"\n\t\"time\"\n)\n\n")
	buf.WriteString("// supressing unused-import errors before goimports is run.\n")
	buf.WriteString("var _ = xml.Name{}\n")
	buf.WriteString("var _ = time.Time{}\n\n")

	buildStructs(ast.Root, &buf, make(map[string]bool))

	outPath := filepath.Join(folderPath, xsdToGoFileName(xsdFile))
	if err := os.WriteFile(outPath, []byte(buf.String()), 0644); err != nil {
		return &metaError{
			FileName: outPath,
			Content:  fmt.Sprintf("failed to write output file: %v", err),
		}
	}
	return nil
}

// =============================================================================
