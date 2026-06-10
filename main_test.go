package main

import (
	"strings"
	"testing"
)

func TestSplitName(t *testing.T) {
	tests := []struct{ in, l1, l2 string }{
		{"JOÃO SILVA", "JOÃO", "SILVA"},
		{"MARIA", "MARIA", ""},
		{"ANA PAULA", "ANA", "PAULA"},
		{"", "", ""},
	}
	for _, tt := range tests {
		l1, l2 := splitName(tt.in)
		if l1 != tt.l1 || l2 != tt.l2 {
			t.Errorf("splitName(%q) = %q, %q; want %q, %q", tt.in, l1, l2, tt.l1, tt.l2)
		}
	}
}

func TestParseFloatDef(t *testing.T) {
	if v := parseFloatDef("8.2", 0); v != 8.2 {
		t.Errorf("got %v, want 8.2", v)
	}
	if v := parseFloatDef("abc", 5.0); v != 5.0 {
		t.Errorf("got %v, want 5.0 (default)", v)
	}
	if v := parseFloatDef("", 3.0); v != 3.0 {
		t.Errorf("got %v, want 3.0 (empty)", v)
	}
	if v := parseFloatDef("  10.5  ", 0); v != 10.5 {
		t.Errorf("got %v, want 10.5 (trimmed)", v)
	}
}

func TestParseIntDef(t *testing.T) {
	if v := parseIntDef("12", 0); v != 12 {
		t.Errorf("got %v, want 12", v)
	}
	if v := parseIntDef("abc", 30); v != 30 {
		t.Errorf("got %v, want 30 (default)", v)
	}
	if v := parseIntDef("  8  ", 0); v != 8 {
		t.Errorf("got %v, want 8 (trimmed)", v)
	}
}

func TestCalcFontSize(t *testing.T) {
	fs := calcFontSize("JOÃO SILVA", 100, 0.7, 6.1)
	if fs <= 0 {
		t.Errorf("calcFontSize returned %v, expected > 0", fs)
	}
}

func TestGenerateScadContent(t *testing.T) {
	cfg := Config{
		Nome:            "JOÃO SILVA",
		DiametroFuro:    8.2,
		EspessuraParede: 2.0,
		EspessuraRelevo: 2.5,
		ComprimentoTubo: 100,
		LarguraTexto:    0.7,
		Fn:              12,
	}
	content := generateScadContent(cfg)
	if !strings.Contains(content, "JOÃO") {
		t.Error("SCAD content missing Lado1")
	}
	if !strings.Contains(content, "SILVA") {
		t.Error("SCAD content missing Lado2")
	}
	if !strings.Contains(content, "BOSL2") {
		t.Error("SCAD content missing BOSL2 include")
	}
}

func TestFindOpenSCAD(t *testing.T) {
	path := findOpenSCAD()
	if path == "" {
		t.Error("OpenSCAD not found (expected nightly)")
	}
}
