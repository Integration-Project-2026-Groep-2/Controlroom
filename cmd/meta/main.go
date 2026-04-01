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

const (
	clReset = "\033[0m"
	clGreen = "\033[32m"
	clRed   = "\033[31m"
	clGray  = "\033[90m"
	clBold  = "\033[1m"
)

// NOTE(nasr): parsing command line arguments for defining the source foulders where the xml files can be found and the destination folder for throwing in the structs
var (
	base = flag.String("path", "pkg/xml/", "path to folder containing xsd files")
	out  = flag.String("out", "pkg/xml/gen/", "path to folder where .go files are written")
)

func main() {
	flag.Parse()

	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatalf("xmlgen: failed to create output directory: %v", err)
	}

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

		name := entry.Name()

		var lexer meta.MetaLexer
		if err := lexer.LoadFile(*base, name); err != nil {
			fmt.Printf("%s  FAIL%s  %s %s%v%s\n", clRed+clBold, clReset, name, clGray, err, clReset)
			continue
		}

		var ast meta.AST
		if err := lexer.Lex(&ast); err != nil {
			fmt.Printf("%s  FAIL%s  %s %s%v%s\n", clRed+clBold, clReset, name, clGray, err, clReset)
			continue
		}

		if err := meta.WriteGoStruct(&ast, *out, name); err != nil {
			fmt.Printf("%s  FAIL%s  %s %s%v%s\n", clRed+clBold, clReset, name, clGray, err, clReset)
			continue
		}

		fmt.Printf("%s  ok  %s  %s\n", clGreen+clBold, clReset, name)
	}
}
