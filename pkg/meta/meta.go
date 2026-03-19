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

// custom errors
// =============================================================================
type MetaError struct {
	FileName string
	Content  string
	Position string
	Line     int
	Column   int
}

func (e *MetaError) Error() string {
	return fmt.Sprintf("error in %s at line %d, column %d: %s", e.FileName, e.Line, e.Column, e.Content)
}
// =============================================================================

// file loading and handling
// =============================================================================
func isXsd(fileName string) bool {
	return filepath.Ext(fileName) == ".xsd"
}

func Load(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, &MetaError{
			FileName: base,
			Content:  fmt.Sprintf("failed to read directory >>> %v <<<", err),
		}
	}

	var contents []string
	for _, file := range entries {
		if !isXsd(file.Name()) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(base, file.Name()))
		if err != nil {
			return nil, &MetaError{
				FileName: file.Name(),
				Content:  fmt.Sprintf("failed to open file >>> %v <<<", err),
			}
		}
		contents = append(contents, string(content))
	}

	return contents, nil

}
