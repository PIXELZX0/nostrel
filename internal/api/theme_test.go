package api

import "testing"

func TestHexColor(t *testing.T) {
	good := []string{"", "#121417", "#F7931A"}
	bad := []string{"#fff", "red", "#12141", "#1214177", "#12141z", "#121417;background:url(x)"}

	for _, v := range good {
		if !hexColor(v) {
			t.Errorf("hexColor(%q) = false, want true", v)
		}
	}
	for _, v := range bad {
		if hexColor(v) {
			t.Errorf("hexColor(%q) = true, want false", v)
		}
	}
}

func TestImageURL(t *testing.T) {
	good := []string{"", "https://cdn.example.com/bg.png", "http://example.com/a.jpg?v=2"}
	bad := []string{
		"javascript:alert(1)",
		"/local/path.png",
		"https://example.com/a.png\") ; background: url(\"evil",
		"https://example.com/a b.png",
		"https://",
	}

	for _, v := range good {
		if !imageURL(v) {
			t.Errorf("imageURL(%q) = false, want true", v)
		}
	}
	for _, v := range bad {
		if imageURL(v) {
			t.Errorf("imageURL(%q) = true, want false", v)
		}
	}
}
