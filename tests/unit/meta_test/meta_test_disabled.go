package meta_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-project-ehb/controlroom/pkg/meta"
)

// ---- helpers ----------------------------------------------------------------

// writeXSD writes content to a temp dir and returns the dir + filename.
func writeXSD(t *testing.T, name, content string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	return dir
}

func lexAndParse(t *testing.T, dir, name string) meta.AST {
	t.Helper()
	var lexer meta.Lexer
	require.NoError(t, lexer.LoadFile(dir, name))
	var ast meta.AST
	require.NoError(t, lexer.Lex(&ast))
	return ast
}

// ---- IsXsd ------------------------------------------------------------------

func TestIsXsd_True(t *testing.T) {
	assert.True(t, meta.IsXsd("heartbeat.xsd"))
	assert.True(t, meta.IsXsd("user_confirmed.xsd"))
}

func TestIsXsd_False(t *testing.T) {
	assert.False(t, meta.IsXsd("heartbeat.go"))
	assert.False(t, meta.IsXsd("heartbeat.xml"))
	assert.False(t, meta.IsXsd("heartbeat"))
	assert.False(t, meta.IsXsd(""))
}

// ---- LoadFile ---------------------------------------------------------------

func TestLoadFile_MissingFile(t *testing.T) {
	var lexer meta.Lexer
	err := lexer.LoadFile("/does/not/exist", "nope.xsd")
	assert.Error(t, err)
}

func TestLoadFile_ValidFile(t *testing.T) {
	dir := writeXSD(t, "simple.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`)
	var lexer meta.Lexer
	err := lexer.LoadFile(dir, "simple.xsd")
	assert.NoError(t, err)
}

// ---- Lex / AST --------------------------------------------------------------

func TestLex_EmptySchema(t *testing.T) {
	dir := writeXSD(t, "empty.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`)
	ast := lexAndParse(t, dir, "empty.xsd")
	assert.NotNil(t, ast.Root)
}

func TestLex_SimpleComplexType(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Heartbeat">
    <xs:sequence>
      <xs:element name="serviceId" type="xs:string"/>
      <xs:element name="timestamp" type="xs:dateTime"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	dir := writeXSD(t, "heartbeat.xsd", xsd)
	ast := lexAndParse(t, dir, "heartbeat.xsd")
	assert.NotNil(t, ast.Root)
	// root should be schema; first child should be complexType
	require.NotNil(t, ast.Root.First)
	assert.Equal(t, meta.TagType("complexType"), ast.Root.First.Tag)
	assert.Equal(t, "Heartbeat", ast.Root.First.Attrs.Name)
}

func TestLex_SimpleTypeWithEnumeration(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:simpleType name="UserRoleType">
    <xs:restriction base="xs:string">
      <xs:enumeration value="ADMIN"/>
      <xs:enumeration value="VISITOR"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	dir := writeXSD(t, "role.xsd", xsd)
	ast := lexAndParse(t, dir, "role.xsd")
	assert.NotNil(t, ast.Root)
	require.NotNil(t, ast.Root.First)
	assert.Equal(t, meta.TagType("simpleType"), ast.Root.First.Tag)
	assert.Equal(t, "UserRoleType", ast.Root.First.Attrs.Name)
}

func TestLex_ProcessingInstructionIgnored(t *testing.T) {
	const xsd = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Ping">
    <xs:sequence>
      <xs:element name="id" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	dir := writeXSD(t, "ping.xsd", xsd)
	ast := lexAndParse(t, dir, "ping.xsd")
	require.NotNil(t, ast.Root.First)
	assert.Equal(t, "Ping", ast.Root.First.Attrs.Name)
}

func TestLex_CommentsIgnored(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <!-- this comment should be ignored -->
  <xs:complexType name="Thing">
    <xs:sequence>
      <xs:element name="value" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	dir := writeXSD(t, "thing.xsd", xsd)
	ast := lexAndParse(t, dir, "thing.xsd")
	require.NotNil(t, ast.Root.First)
	assert.Equal(t, "Thing", ast.Root.First.Attrs.Name)
}

func TestLex_MultipleComplexTypes(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="TypeA">
    <xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="TypeB">
    <xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence>
  </xs:complexType>
</xs:schema>`
	dir := writeXSD(t, "multi.xsd", xsd)
	ast := lexAndParse(t, dir, "multi.xsd")
	require.NotNil(t, ast.Root.First)
	assert.Equal(t, "TypeA", ast.Root.First.Attrs.Name)
	require.NotNil(t, ast.Root.First.Next)
	assert.Equal(t, "TypeB", ast.Root.First.Next.Attrs.Name)
}

// ---- WriteGoStruct (code generation output) ---------------------------------

func TestWriteGoStruct_NilAST(t *testing.T) {
	err := meta.WriteGoStruct(nil, t.TempDir(), "empty.xsd")
	assert.Error(t, err)
}

func TestWriteGoStruct_EmptyAST(t *testing.T) {
	ast := &meta.AST{}
	err := meta.WriteGoStruct(ast, t.TempDir(), "empty.xsd")
	assert.Error(t, err)
}

func TestWriteGoStruct_ProducesGoFile(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Heartbeat">
    <xs:sequence>
      <xs:element name="serviceId" type="xs:string"/>
      <xs:element name="timestamp" type="xs:dateTime"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	dir := writeXSD(t, "heartbeat.xsd", xsd)
	outDir := t.TempDir()
	err := meta.WriteGoStruct(new(lexAndParse(t, dir, "heartbeat.xsd")), outDir, "heartbeat.xsd")
	require.NoError(t, err)

	// file should exist
	outPath := filepath.Join(outDir, "heartbeat.go")
	_, err = os.Stat(outPath)
	assert.NoError(t, err, "expected heartbeat.go to be created")
}

func TestWriteGoStruct_OutputContainsPackageGen(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Ping">
    <xs:sequence>
      <xs:element name="id" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	dir := writeXSD(t, "ping.xsd", xsd)
	outDir := t.TempDir()
	require.NoError(t, meta.WriteGoStruct(new(lexAndParse(t, dir, "ping.xsd")), outDir, "ping.xsd"))

	content, err := os.ReadFile(filepath.Join(outDir, "ping.go"))
	require.NoError(t, err)

	src := string(content)
	assert.True(t, strings.Contains(src, "package gen"), "expected 'package gen'")
	assert.True(t, strings.Contains(src, "DO NOT EDIT"), "expected DO NOT EDIT header")
	assert.True(t, strings.Contains(src, `type Ping struct`), "expected Ping struct")
	assert.True(t, strings.Contains(src, `xml:"Ping"`), "expected xml tag")
}

func TestWriteGoStruct_EnumConstantsEmitted(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:simpleType name="Color">
    <xs:restriction base="xs:string">
      <xs:enumeration value="RED"/>
      <xs:enumeration value="GREEN"/>
      <xs:enumeration value="BLUE"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	dir := writeXSD(t, "color.xsd", xsd)
	outDir := t.TempDir()
	require.NoError(t, meta.WriteGoStruct(new(lexAndParse(t, dir, "color.xsd")), outDir, "color.xsd"))

	content, err := os.ReadFile(filepath.Join(outDir, "color.go"))
	require.NoError(t, err)

	src := string(content)
	assert.True(t, strings.Contains(src, `type Color string`))
	assert.True(t, strings.Contains(src, `ColorRED`))
	assert.True(t, strings.Contains(src, `ColorGREEN`))
	assert.True(t, strings.Contains(src, `ColorBLUE`))
	assert.True(t, strings.Contains(src, `"RED"`))
}

func TestWriteGoStruct_HyphenatedFilenameBecomesUnderscore(t *testing.T) {
	const xsd = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Foo">
    <xs:sequence><xs:element name="x" type="xs:string"/></xs:sequence>
  </xs:complexType>
</xs:schema>`
	dir := writeXSD(t, "my-type.xsd", xsd)
	outDir := t.TempDir()
	require.NoError(t, meta.WriteGoStruct(new(lexAndParse(t, dir, "my-type.xsd")), outDir, "my-type.xsd"))

	// hyphens must become underscores in the output filename
	_, err := os.Stat(filepath.Join(outDir, "my_type.go"))
	assert.NoError(t, err, "expected my_type.go (hyphen → underscore)")
}
