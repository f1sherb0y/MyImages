// MyImages is a fast, compact picture viewer.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"myimages/internal/app"
)

const usage = `myimages [options] [file or folder]

  Shows a picture, with the other pictures of its folder to go through.

      --bench            time the pictures of the folder, then quit
      --screenshot FILE  write a picture of the window to FILE, then quit
      --ephemeral        leave the settings alone; run beside another instance
  -h, --help   -v, --version
`

func main() {
	started := time.Now()
	profile()
	var opt app.Options
	var paths []string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "-h", "--help", "/?":
			attachConsole()
			fmt.Print(usage)
			return
		case "-v", "--version":
			attachConsole()
			fmt.Println("myimages", app.Version())
			return
		case "--bench":
			opt.Bench = true
		case "--ephemeral":
			opt.Ephemeral = true
		case "--screenshot":
			if i+1 < len(args) {
				i++
				opt.Screenshot = args[i]
			}
		default:
			if arg == "" || arg[0] == '-' {
				continue
			}
			if abs, err := filepath.Abs(arg); err == nil {
				arg = abs
			}
			paths = append(paths, arg)
		}
	}
	if opt.Bench || opt.Screenshot != "" {
		attachConsole()
	}
	if err := app.Run(paths, opt, started); err != nil {
		attachConsole()
		fmt.Fprintln(os.Stderr, "myimages:", err)
		os.Exit(1)
	}
}
