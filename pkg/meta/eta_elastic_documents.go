// =============================================================================
// based on the generated go structs we want to generated document structs
// for more performant indexing to elastic, this way we can skip an entire json
// decoding part
// =============================================================================

package meta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// 0. Design the struct that will be needed to be sent to kibana
// 1. Designate the destination for sending the document struct [folder, file]
// 2. Define the source
// 3. Load the go structs
// 4. Define an interface for that generation?
// 6. Selecting what fields we want to send and which ones not?

// returns an array of fiels that we want to pass?
func () SelectFields() []int8 {

	// TODO(nasr): interface for allowing which fields to pass and which ones not
}

func () LoadGeneratedGoStructs() {

}

func () WriteGoDocumentStruct() {

}
