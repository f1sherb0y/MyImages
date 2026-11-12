package app

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"myimages/internal/codec"
	"myimages/internal/shell"
	"myimages/locales"
)

// dialogs builds what shows over the window: a question, the name
// asked for, the settings.
func (w *Window) dialogs(c *ui.Context) {
	if q := w.ask; q != nil {
		switch ui.AlertDialog(c, &q.open, q.title, q.message, locales.Cancel(), q.confirm) {
		case 1:
			w.ask = nil
			q.yes()
		case 0, ui.AlertDismissed:
			w.ask = nil
		}
	}
	ui.Modal(c.Key("rename"), &w.renaming, func() {
		ui.Text(c, locales.Rename()).Bold()
		if ui.TextInput(c, &w.newName).Width(320).AutoFocus().Submitted() {
			w.applyRename()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Clicked() {
				w.renaming = false
			}
			if ui.PrimaryButton(c, locales.Rename()).Clicked() {
				w.applyRename()
			}
		})
	})
	ui.Modal(c.Key("settings"), &w.settings, func() { w.settingsPage(c) })
}

// option is a choice of a setting: what the file says, and what the
// user reads.
type option struct{ id, label string }

// choose is a row of the settings choosing one of a few options.
func (w *Window) choose(c *ui.Context, label string, value *string, options ...option) bool {
	changed := false
	settingRow(c, label, func() {
		index := 0
		labels := make([]string, len(options))
		for i, o := range options {
			labels[i] = o.label
			if o.id == *value {
				index = i
			}
		}
		if ui.Segmented(c.Key(label), &index, labels...).Label(label).Changed() {
			*value = options[index].id
			changed = true
		}
	})
	return changed
}

// flag is a row of the settings turning something on and off.
func (w *Window) flag(c *ui.Context, label string, value *bool) bool {
	changed := false
	settingRow(c, label, func() {
		if ui.Switch(c.Key(label), value).Label(label).Changed() {
			changed = true
		}
	})
	return changed
}

func settingRow(c *ui.Context, label string, control func()) {
	ui.Row(c).AlignItems(ui.Center).Gap(16).MinHeight(28).Children(func() {
		ui.Text(c, label).Grow(1).Shrink(1)
		control()
	})
}

// settingsPage is what the settings show.
func (w *Window) settingsPage(c *ui.Context) {
	a := w.app
	set := &a.set
	th := c.Theme()
	changed := false
	ui.Column(c).Width(460).Gap(4).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.Settings()).FontSize(15).Bold().Grow(1)
			ui.Text(c, "MyImages "+Version()).TextColor(th.TextMuted)
		})
		ui.Box(c).Height(6)
		if w.choose(c, locales.SetTheme(), &set.Theme,
			option{"system", locales.ThemeSystem()}, option{"light", locales.ThemeLight()}, option{"dark", locales.ThemeDark()}) {
			a.applyTheme()
			changed = true
		}
		w.language(c)
		changed = w.choose(c, locales.SetBackground(), &set.Background,
			option{"theme", locales.BackgroundTheme()}, option{"black", locales.BackgroundBlack()}, option{"white", locales.BackgroundWhite()}) || changed
		if w.choose(c, locales.ZoomMode(), &set.Zoom,
			option{"auto", locales.ZoomAutoShort()}, option{"fit", locales.ZoomFitShort()}, option{"fill", locales.ZoomFillShort()}, option{"actual", "100%"}) {
			w.fit()
			changed = true
		}
		wheel := "zoom"
		if set.WheelBrowses {
			wheel = "browse"
		}
		if w.choose(c, locales.SetWheel(), &wheel, option{"zoom", locales.WheelZooms()}, option{"browse", locales.WheelBrowses()}) {
			set.WheelBrowses = wheel == "browse"
			changed = true
		}
		if w.flag(c, locales.SetSmooth(), &set.Smooth) {
			// The pictures decoded show as they were made: made again.
			for _, win := range a.windows {
				win.reload()
			}
			changed = true
		}
		changed = w.flag(c, locales.KeepZoom(), &set.KeepZoom) || changed
		changed = w.flag(c, locales.SetLoop(), &set.Loop) || changed
		if w.flag(c, locales.SetHidden(), &set.ShowHidden) {
			w.relist()
			changed = true
		}
		changed = w.flag(c, locales.SetConfirmDelete(), &set.ConfirmDelete) || changed
		changed = w.flag(c, locales.SetOneWindow(), &set.OneWindow) || changed
		settingRow(c, locales.SetSlideSeconds(fmt.Sprintf("%.0f", set.SlideSeconds)), func() {
			if ui.Slider(c.Key("slide"), &set.SlideSeconds, 1, 30).Step(1).Width(180).Changed() {
				changed = true
			}
		})
		changed = w.flag(c, locales.SetAutoUpdate(), &set.AutoUpdate) || changed
		ui.Box(c).Height(6)
		ui.Text(c, locales.DefaultFormats()).FontSize(11).TextColor(th.TextMuted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			label := locales.MakeDefault()
			if a.defaultsBusy {
				label = locales.MakingDefault()
			}
			if ui.PrimaryButton(c.Key("make-default"), label).Disabled(a.defaultsBusy).Clicked() {
				a.makeDefault(shell.CommonImageExtensions())
			}
			if ui.Button(c.Key("defaults-all"), locales.MakeDefaultAll()).Disabled(a.defaultsBusy).Clicked() {
				a.makeDefault(codec.Extensions())
			}
		})
		if a.defaultsMessage != "" {
			ui.Text(c, a.defaultsMessage).FontSize(11).Tooltip(a.defaultsDetail)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			switch {
			case a.updateReady != "":
				if ui.PrimaryButton(c.Key("restart-now"), locales.UpdateRestart()).Clicked() {
					a.restart()
				}
			case a.updating:
				ui.Text(c, locales.UpdateChecking()).TextColor(th.TextMuted)
			default:
				if ui.Button(c.Key("check"), locales.UpdateCheck()).Clicked() {
					was := set.AutoUpdate
					set.AutoUpdate = true
					a.checkUpdate()
					set.AutoUpdate = was
				}
			}
			if ui.PrimaryButton(c.Key("done"), locales.Done()).Clicked() {
				w.settings = false
			}
		})
	})
	if changed {
		a.saveSoon()
		a.invalidate()
	}
}

// language is the row of the settings choosing the language shown.
func (w *Window) language(c *ui.Context) {
	langs := w.app.langs
	if len(langs) < 2 {
		return
	}
	names := map[string]string{"en": "English", "zh-Hans": "简体中文"}
	system := locales.LanguageSystem()
	options := []string{system}
	for _, tag := range langs {
		name := names[tag]
		if name == "" {
			name = tag
		}
		options = append(options, name)
	}
	chosen := system
	if tag := w.app.lang; tag != "" {
		if i := slices.Index(langs, tag); i >= 0 {
			chosen = options[i+1]
		}
	}
	settingRow(c, locales.SetLanguage(), func() {
		if ui.Select(c.Key("language"), &chosen, options).Width(180).Changed() {
			tag := ""
			if i := slices.Index(options, chosen); i > 0 {
				tag = langs[i-1]
			}
			w.app.lang = tag
			mygo.App.SetLocale(tag)
			w.app.invalidate()
		}
	})
}
