package startup

import "testing"

func TestCommandQuotesPath(t *testing.T) {
	got, err := Command(`C:\Program Files\Smart Desktop\SmartDesktop.exe`)
	if err != nil || got != `"C:\Program Files\Smart Desktop\SmartDesktop.exe" --background` {
		t.Fatal(got, err)
	}
	for _, s := range []string{"", `C:\bad"path.exe`, "bad\x00path"} {
		if _, err := Command(s); err == nil {
			t.Fatal(s)
		}
	}
}
