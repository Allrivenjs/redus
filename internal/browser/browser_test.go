package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileFor(t *testing.T) {
	dir := t.TempDir()
	state := `{"profile":{"info_cache":{
		"Default":{"user_name":"personal@gmail.com"},
		"Profile 3":{"user_name":"Jaime@Adipa.co"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "Local State"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	o := Opener{UserDataDir: dir, Profiles: map[string]string{"forzado@x.com": "Profile 9"}}
	cases := map[string]string{
		"jaime@adipa.co":     "Profile 3", // sin distinguir mayúsculas
		"personal@gmail.com": "Default",
		"forzado@x.com":      "Profile 9", // la config manda
		"otra@x.com":         "",
	}
	for acc, want := range cases {
		if got := o.ProfileFor(acc); got != want {
			t.Errorf("%s: %q, esperaba %q", acc, got, want)
		}
	}
}

func TestWithAuthUser(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://meet.google.com/abc-defg-hij", "https://meet.google.com/abc-defg-hij?authuser=a%40b.co"},
		{"https://www.google.com/calendar/event?eid=xyz", "https://www.google.com/calendar/event?authuser=a%40b.co&eid=xyz"},
		{"https://meet.google.com/x?authuser=1", "https://meet.google.com/x?authuser=1"},
		{"https://zoom.us/j/123", "https://zoom.us/j/123"},
	}
	for _, c := range cases {
		if got := WithAuthUser(c.in, "a@b.co"); got != c.want {
			t.Errorf("%s -> %s, esperaba %s", c.in, got, c.want)
		}
	}
}

func TestSchemeRoundTrip(t *testing.T) {
	link := "https://meet.google.com/abc?x=1&y=2"
	l, a, ok := ParseSchemeURL(SchemeURL(link, "jaime@adipa.co"))
	if !ok || l != link || a != "jaime@adipa.co" {
		t.Fatalf("ida y vuelta: %q %q %v", l, a, ok)
	}
	for _, bad := range []string{
		SchemeURL("javascript:alert(1)", ""),
		SchemeURL("file:///etc/passwd", ""),
		"redus://otro?url=https://x.com",
		"https://meet.google.com/abc",
	} {
		if _, _, ok := ParseSchemeURL(bad); ok {
			t.Errorf("debería rechazar %s", bad)
		}
	}
}
