// Regtry registers a copy of MyImages with the system, as the program
// does when it starts, and tells which extensions became its own.
//
//	regtry MyImages.exe
package main

import (
	"fmt"
	"os"

	"myimages/internal/codec"
	"myimages/internal/shell"
)

func main() {
	claimed, err := shell.RegisterDefaults(os.Args[1], codec.Extensions())
	fmt.Println("error:", err)
	fmt.Println("now the program's:", len(claimed), claimed)
}
