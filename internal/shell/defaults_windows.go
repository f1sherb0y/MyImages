package shell

import (
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// progID names the kind of file an extension is, as the program's
// installer registers it.
func progID(ext string) string { return "dev.myimages.viewer." + ext }

// RegisterDefaults tells the system which kinds of files the program at
// exe opens, for the current user, so that the system's settings list it
// among the programs that can be the default for them. An extension that
// nothing opens yet becomes the program's, as an installer makes a
// format its own; where the user chose a program, or another claimed the
// extension, nothing changes: only the user changes that, in the
// settings. It returns the extensions it made the program's. Every write
// is the same each time, so it is safe to run again.
func RegisterDefaults(exe string, exts []string) (claimed []string, err error) {
	const capabilities = `Software\MyImages\Capabilities`
	set := func(path, name, value string) error {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		return k.SetStringValue(name, value)
	}
	command := `"` + exe + `" "%1"`
	for _, ext := range exts {
		id := progID(ext)
		// The kind of file: what opens it, its icon, its name.
		if err := set(`Software\Classes\`+id+`\shell\open\command`, "", command); err != nil {
			return claimed, err
		}
		_ = set(`Software\Classes\`+id, "", strings.ToUpper(ext)+" Image")
		_ = set(`Software\Classes\`+id+`\DefaultIcon`, "", exe+",0")
		_ = set(`Software\Classes\`+id+`\shell\open`, "FriendlyAppName", "MyImages")
		if unclaimed(ext) && set(`Software\Classes\.`+ext, "", id) == nil {
			claimed = append(claimed, ext)
		}
		// Offered for the extension, and claimed as one the program
		// opens.
		_ = set(`Software\Classes\.`+ext+`\OpenWithProgids`, id, "")
		if err := set(capabilities+`\FileAssociations`, "."+ext, id); err != nil {
			return claimed, err
		}
	}
	_ = set(capabilities, "ApplicationName", "MyImages")
	_ = set(capabilities, "ApplicationDescription", "A fast, compact picture viewer")
	_ = set(`Software\Classes\Applications\`+exeName(exe)+`\shell\open\command`, "", command)
	_ = set(`Software\Classes\Applications\`+exeName(exe), "FriendlyAppName", "MyImages")
	return claimed, set(`Software\RegisteredApplications`, "MyImages", capabilities)
}

// unclaimed reports whether nothing opens an extension yet: the user
// chose no program for it, and no other program named its kind of file
// as the extension's, for the user or for the machine.
func unclaimed(ext string) bool {
	if DefaultFor(ext) != "" {
		return false
	}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `Software\Classes\.`+ext, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		kind, _, _ := k.GetStringValue("")
		k.Close()
		if kind != "" && !strings.EqualFold(kind, progID(ext)) {
			return false
		}
	}
	return true
}

func exeName(exe string) string {
	if i := strings.LastIndexAny(exe, `\/`); i >= 0 {
		return exe[i+1:]
	}
	return exe
}

// DefaultApps opens the system's settings at the page of the program,
// where the user makes it the default for the kinds of files it opens:
// Windows lets no program do that itself.
func DefaultApps() error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", "ms-settings:defaultapps?registeredAppUser=MyImages").Start()
}

// DefaultFor reports the kind of file the user chose to open an
// extension with, "" for none.
func DefaultFor(ext string) string {
	for _, name := range []string{`UserChoiceLatest\ProgId`, "UserChoice"} {
		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.`+ext+`\`+name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		id, _, err := k.GetStringValue("ProgId")
		k.Close()
		if err == nil && id != "" {
			return id
		}
	}
	return ""
}

// IsDefaultFor reports whether the program is the default for an
// extension.
func IsDefaultFor(ext string) bool { return strings.EqualFold(DefaultFor(ext), progID(ext)) }

// A program cannot make itself the default. The system keeps the user's
// choice for an extension signed, and where a program removes it to have
// the extension's own kind count, as programs once did, Windows 11 puts
// its own default back at once (tried on build 26300: .jpg went to
// Photos). The settings' page is the way.
