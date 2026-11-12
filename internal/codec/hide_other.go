//go:build !windows

package codec

import "os/exec"

func hideWindow(*exec.Cmd) {}
