// Package config keeps what the user set, in a file of the user's
// configuration directory. It knows nothing of the interface.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings are the user's preferences.
type Settings struct {
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`
	// Background is what shows behind the picture: "theme", "black",
	// "white" or "checker".
	Background string `json:"background"`
	// Zoom is how a picture first shows: "auto" (whole, never enlarged),
	// "fit" (whole, enlarged to the window), "fill" or "actual".
	Zoom string `json:"zoom"`
	// KeepZoom keeps the zoom and the place from one picture to the
	// next.
	KeepZoom bool `json:"keepZoom"`
	// WheelBrowses makes the wheel go through the pictures, and zoom
	// with Ctrl held; otherwise the other way around.
	WheelBrowses bool `json:"wheelBrowses"`
	// Smooth blends the pixels of a picture shown enlarged; otherwise
	// each shows as a square.
	Smooth bool `json:"smooth"`
	// Loop goes from the last picture of a folder to the first.
	Loop bool `json:"loop"`
	// Toolbar, Thumbnails, Status and Info show the parts of the window.
	Toolbar    bool `json:"toolbar"`
	Thumbnails bool `json:"thumbnails"`
	Status     bool `json:"status"`
	Info       bool `json:"info"`
	// ThumbSize is the height of the thumbnails, in DIPs.
	ThumbSize float32 `json:"thumbSize"`
	// SortBy is "name", "date", "size" or "type".
	SortBy         string `json:"sortBy"`
	SortDescending bool   `json:"sortDescending"`
	// ShowHidden lists the files the system hides.
	ShowHidden bool `json:"showHidden"`
	// SlideSeconds is how long a slideshow shows each picture.
	SlideSeconds float64 `json:"slideSeconds"`
	// ConfirmDelete asks before moving a file to the Recycle Bin.
	ConfirmDelete bool `json:"confirmDelete"`
	// OneWindow shows a picture opened from elsewhere in the window that
	// is open, rather than in a new one.
	OneWindow bool `json:"oneWindow"`
	// AutoUpdate looks for newer versions, and downloads them.
	AutoUpdate bool `json:"autoUpdate"`
	// Registered is the version that last told the system which files
	// the program opens.
	Registered string `json:"registered,omitempty"`
	// ViewW and ViewH are the size the picture area last had, in pixels:
	// what the first picture is decoded for before the window exists.
	ViewW int `json:"viewW,omitempty"`
	ViewH int `json:"viewH,omitempty"`
}

// Defaults are the settings of a first run.
func Defaults() Settings {
	return Settings{
		Theme:         "system",
		Background:    "theme",
		Zoom:          "auto",
		Loop:          true,
		Toolbar:       true,
		Status:        true,
		ThumbSize:     64,
		SortBy:        "name",
		SlideSeconds:  4,
		ConfirmDelete: false,
		OneWindow:     true,
		AutoUpdate:    true,
	}
}

// Dir is the directory the files are kept in, made when missing.
// MYIMAGES_CONFIG_DIR names another, for tests.
func Dir() (string, error) {
	dir := os.Getenv("MYIMAGES_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "MyImages")
	}
	return dir, os.MkdirAll(dir, 0o755)
}

// Load reads the settings; the defaults where there are none.
func Load() Settings {
	s := Defaults()
	dir, err := Dir()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return s
	}
	if json.Unmarshal(data, &s) != nil {
		return Defaults()
	}
	if s.ThumbSize < 32 || s.ThumbSize > 256 {
		s.ThumbSize = 64
	}
	if s.SlideSeconds < 0.5 {
		s.SlideSeconds = 4
	}
	return s
}

// Save writes the settings, replacing the file whole: a crash while
// writing leaves the old one.
func (s Settings) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
