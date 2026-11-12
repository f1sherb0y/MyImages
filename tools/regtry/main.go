// Regtry registers a copy of MyImages with the system's default apps and
// opens the settings at its page: what the settings' button does.
package main

import (
	"fmt"
	"os"

	"myimages/internal/codec"
	"myimages/internal/shell"
)

func main() {
	exe := os.Args[1]
	fmt.Println("register:", shell.RegisterDefaults(exe, codec.Extensions()), len(codec.Extensions()), "extensions")
	fmt.Println("jpg default:", shell.DefaultFor("jpg"), shell.IsDefaultFor("jpg"))
	if len(os.Args) > 2 {
		fmt.Println("settings:", shell.DefaultApps())
	}
}
