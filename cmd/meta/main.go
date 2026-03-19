package main

import (
	"fmt"
	"log"

	"integration-project-ehb/controlroom/pkg/meta"
)

func main() {
	base := "pkg/xml/"
	output, err := meta.Load(base)
	if err != nil {
		log.Fatalf("xmlgen failed: %v", err)
	}

	fmt.Println(output)
}
