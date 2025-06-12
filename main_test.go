package main

import (
	"os"
	"testing"
)

func TestLoadConfig_ValidFile(t *testing.T) {
	content := `{
		"server": "irc.example.com:6667",
		"nick": "testbot",
		"channel": "#testchan",
		"nickserv": { "password": "secret" }
	}`
	tmpfile, err := os.CreateTemp("", "config*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpfile.Close()

	bot := &CinemaBot{}
	err = bot.loadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if bot.config.Server != "irc.example.com:6667" {
		t.Errorf("expected server, got %s", bot.config.Server)
	}
	if bot.config.Nick != "testbot" {
		t.Errorf("expected nick, got %s", bot.config.Nick)
	}
	if bot.config.Channel != "#testchan" {
		t.Errorf("expected channel, got %s", bot.config.Channel)
	}
	if bot.config.NickServ.Password != "secret" {
		t.Errorf("expected password, got %s", bot.config.NickServ.Password)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	bot := &CinemaBot{}
	err := bot.loadConfig("nonexistent_file.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	bot := &CinemaBot{}
	err := bot.loadConfig("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if bot.config.Server == "" || bot.config.Nick == "" || bot.config.Channel == "" {
		t.Error("expected default config values to be set")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	content := `{invalid json}`
	tmpfile, err := os.CreateTemp("", "config*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpfile.Close()

	bot := &CinemaBot{}
	err = bot.loadConfig(tmpfile.Name())
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseArgs_Simple(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(";showtime -list")
	expected := []string{";showtime", "-list"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_Quoted(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -create -title="My Movie" -id="abc"`)
	expected := []string{";showtime", "-create", "-title=My Movie", "-id=abc"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_EscapedQuotes(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title="A \"Great\" Movie"`)
	expected := []string{";showtime", "-title=A \"Great\" Movie"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_EscapedSpace(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title=My\ Movie`)
	expected := []string{";showtime", "-title=My Movie"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_Empty(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs("")
	expected := []string{}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_LeadingTrailingSpaces(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs("   ;showtime   -list   ")
	expected := []string{";showtime", "-list"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_MultipleQuotedArgs(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title="Movie One" -id="abc" -desc="A \"fun\" night"`)
	expected := []string{";showtime", "-title=Movie One", "-id=abc", "-desc=A \"fun\" night"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_OnlySpaces(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs("     ")
	expected := []string{}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_TabSeparated(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs("\t;showtime\t-list\t")
	expected := []string{";showtime", "-list"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_UnclosedQuote(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title="Unclosed`)
	expected := []string{";showtime", "-title=Unclosed"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_EscapedBackslash(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title=Movie\\Night`)
	expected := []string{";showtime", "-title=Movie\\Night"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_ComplexMix(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title="A \"Great\" Movie" -id=abc\ 123 -desc="Fun\tNight"`)
	expected := []string{";showtime", "-title=A \"Great\" Movie", "-id=abc 123", "-desc=Fun\tNight"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_QuoteInsideUnquoted(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title=Movie"Night"`)
	expected := []string{";showtime", "-title=MovieNight"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_ConsecutiveSpaces(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(";showtime    -list    -foo   ")
	expected := []string{";showtime", "-list", "-foo"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_OnlyQuotes(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`""`)
	expected := []string{""}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestParseArgs_QuoteWithEscapedBackslash(t *testing.T) {
	bot := &CinemaBot{}
	args := bot.parseArgs(`;showtime -title="Movie with \\backslash"`)
	expected := []string{";showtime", "-title=Movie with \\backslash"}
	if !equalStringSlices(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

// Helper for comparing slices
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
