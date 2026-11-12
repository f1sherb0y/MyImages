// MyImages is a fast, compact picture viewer.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"myimages/internal/app"
	"myimages/internal/assochash"
	"myimages/internal/codec"
	"myimages/internal/shell"
)

//go:embed LICENSE
var license string

const usage = `myimages [options] [file or folder]

  Shows a picture, with the other pictures of its folder to go through.

      --set-default      set common image defaults for this user, then quit
      --set-default-all  set all supported image defaults, then quit
      --default-status   print effective common image defaults, then quit
      --licenses         print app and association-code license notices
      --bench            time the pictures of the folder, then quit
      --screenshot FILE  write a picture of the window to FILE, then quit
      --ephemeral        leave the settings alone; run beside another instance
  -h, --help   -v, --version
`

func main() {
	started := time.Now()
	profile()
	var opt app.Options
	var setDefault, setDefaultAll, defaultStatus bool
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
		case "--licenses":
			attachConsole()
			fmt.Print("MyImages\n\n", license, "\nUserChoiceLatestHash (cssxn)\n\n", assochash.License)
			return
		case "--set-default":
			setDefault = true
		case "--set-default-all":
			setDefault, setDefaultAll = true, true
		case "--default-status":
			defaultStatus = true
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
	if setDefault || defaultStatus {
		attachConsole()
		if setDefault {
			exe, err := os.Executable()
			var result shell.DefaultResult
			if err == nil {
				exts := shell.CommonImageExtensions()
				if setDefaultAll {
					exts = codec.Extensions()
				}
				result, err = shell.SetDefaults(exe, exts)
			}
			fmt.Printf("Verified defaults: %v\n", result.Succeeded)
			if err != nil {
				fmt.Fprintln(os.Stderr, "myimages:", err)
				os.Exit(1)
			}
		}
		if defaultStatus {
			failed := false
			for _, ext := range shell.CommonImageExtensions() {
				id, err := shell.EffectiveDefaultFor(ext)
				if err != nil {
					fmt.Fprintf(os.Stderr, ".%s: %v\n", ext, err)
					failed = true
				} else {
					fmt.Printf(".%s: %s\n", ext, id)
				}
			}
			if failed {
				os.Exit(1)
			}
		}
		return
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
