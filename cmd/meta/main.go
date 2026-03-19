// =============================================================================
// xmlgen — generates Go structs from XSD schema files
// author: abdellah el morabit
// =============================================================================

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"path/filepath"

	"integration-project-ehb/controlroom/pkg/meta"
)

// handles command line arguments for passing in a root folder for finding xsd files to parse
var (
	base = flag.String("path", "pkg/xml/", "path to folder containing xsd files")
)

func main() {

	// parse command line arguments
	flag.Parse()

	entries, err := os.ReadDir(*base)
	if err != nil {

		fmt.Errorf("error: %v", err)
	}

	for _, file := range entries {

		var lexer meta.MetaLexer

		output, err := lexer.LoadFile(*base, file.Name())

		if err != nil {
			log.Fatalf("xmlgen failed: %v", err)
		}

		if !meta.IsXsd(filepath.Join(file.Name())) {
			continue
		}

		tokens := meta.Lex(output)

		for _, tok := range tokens {
			fmt.Println(string(tok.Lexeme))
		}
	}

}
