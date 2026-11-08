package stacklog

import (
	"strings"
	"testing"
)

// An unknown name resolves to the default preset rather than erroring. That is
// the chosen behaviour, so it is asserted rather than assumed.
func TestUnknownPresetFallsBackToDefault(t *testing.T) {
	l := CreateLogger(Config{Style: Style{Preset: "no-such-preset"}})

	if l.Config.Style.Level[3] != defaultPreset().Level[3] {
		t.Errorf("unknown preset did not fall back to default: got %v", l.Config.Style.Level[3])
	}
}

// A single component override stands against a full preset. The rest of the
// preset is untouched.
func TestSingleComponentOverrideWinsAgainstPreset(t *testing.T) {
	c := Config{Style: Style{
		Preset: PresetBadge,
		Level:  [MaxLevel]CompStyle{NewStyle().Foreground("#D75F5F")},
	}}
	l := CreateLogger(c)

	if l.Config.Style.Level[0] != c.Style.Level[0] {
		t.Error("the caller's component was replaced by the preset")
	}

	if l.Config.Style.Level[1] != badgePreset().Level[1] {
		t.Error("the preset did not fill a component left unset")
	}
}

// A plain preset writes no escape sequence at all. Without the conditional reset
// in addPostStyle this would still emit \x1b[0m after every component.
func TestPlainPresetEmitsNoANSI(t *testing.T) {
	l := CreateLogger(Config{Style: Style{Preset: PresetPlain}})

	l.Buffer.Reset()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.Buffer.WriteNL()

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	for i := 0; i < len(got); i++ {
		if got[i] == 0x1b {
			t.Errorf("escape byte at %d in %q", i, got)
			return
		}
	}

	if got != "ERRO hi\n" {
		t.Errorf("got %q, want %q", got, "ERRO hi\n")
	}
}

// The logger prefix is styled through Style.Prefix. Before this it was hardcoded
// to grayFGStyle, so a preset could not reach it.
func TestLoggerPrefixComesFromThePreset(t *testing.T) {
	l := CreateLogger(Config{Style: Style{Preset: PresetHighContrast}})

	if l.Config.Style.Prefix != highContrastPreset().Prefix {
		t.Error("the prefix was not taken from the preset")
	}

	l = l.WithPrefix("http")

	l.Buffer.Reset()
	l.addLoggerPrefix()

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if got == "" {
		t.Fatal("nothing rendered")
	}

	// Asserted by the colour rather than by the whole byte sequence, so the
	// redundant re-emission in addPostStyle does not make this test brittle.
	if !strings.Contains(got, "38;5;250") {
		t.Errorf("prefix did not use the preset colour: %q", got)
	}

	if strings.Contains(got, "38;5;242") {
		t.Errorf("prefix still uses the hardcoded grey: %q", got)
	}
}
