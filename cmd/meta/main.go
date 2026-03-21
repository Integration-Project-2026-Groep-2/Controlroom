// =============================================================================
// xmlgen — generates Go structs from XSD schema files
// author: abdellah el morabit
// =============================================================================
package main

import (
	"flag"
	"fmt"
	"integration-project-ehb/controlroom/pkg/meta"
	"log"
	"os"
)

var (
	base = flag.String("path", "pkg/xml/", "path to folder containing xsd files")
)

func main() {
	flag.Parse()

	entries, err := os.ReadDir(*base)
	if err != nil {
		log.Fatalf("xmlgen: failed to read directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !meta.IsXsd(entry.Name()) {
			continue
		}

		var lexer meta.MetaLexer
		if err := lexer.LoadFile(*base, entry.Name()); err != nil {
			log.Fatalf("xmlgen: %v", err)
		}

		lexer.Lex()
		for _, tok := range lexer.Tokens {
			fmt.Println(string(tok.Lexeme))
		}
		fmt.Println("//////////////////////////////////////////////////")
	}
}
