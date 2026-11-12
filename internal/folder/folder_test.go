package folder

import (
	"slices"
	"testing"
)

func TestNatural(t *testing.T) {
	names := []string{"img10.jpg", "img9.jpg", "a.png", "img009.jpg", "img1.jpg", "b2", "b10", "b02"}
	slices.SortFunc(names, natural)
	want := []string{"a.png", "b2", "b02", "b10", "img1.jpg", "img9.jpg", "img009.jpg", "img10.jpg"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}
