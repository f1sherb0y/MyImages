package app

import (
	"os"
	"strconv"

	"github.com/egoist/mygo"
	"myimages/internal/shell"
	"myimages/locales"
)

// makeDefault is only called by the settings button. Opening an image,
// installing an update or restarting the application never takes over defaults.
func (a *App) makeDefault(exts []string) {
	if a.defaultsBusy {
		return
	}
	a.defaultsBusy = true
	a.defaultsMessage, a.defaultsDetail = "", ""
	a.invalidate()
	go func() {
		var result shell.DefaultResult
		exe, err := os.Executable()
		if err == nil {
			result, err = shell.SetDefaults(exe, exts)
		}
		mygo.RunOnMain(func() {
			a.defaultsBusy = false
			if err == nil {
				a.defaultsMessage = locales.DefaultsDone()
			} else {
				a.defaultsMessage = locales.DefaultsPartial(strconv.Itoa(len(result.Succeeded)), strconv.Itoa(len(exts)))
				a.defaultsDetail = err.Error()
			}
			a.invalidate()
		})
	}()
}
