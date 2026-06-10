package main

import (
	"context"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestSplitName(t *testing.T) {
	tests := []struct {
		in     string
		l1, l2 string
	}{
		{"JOÃO SILVA", "JOÃO", "SILVA"},
		{"MARIA", "MARIA", ""},
		{"ANA PAULA", "ANA", "PAULA"},
		{"", "", ""},
		{"  espaços  ", "espaços", ""},
		{" três palavras aqui", "três", "palavras"},
	}
	for _, tt := range tests {
		l1, l2 := splitName(tt.in)
		if l1 != tt.l1 || l2 != tt.l2 {
			t.Errorf("splitName(%q) = %q, %q; want %q, %q", tt.in, l1, l2, tt.l1, tt.l2)
		}
	}
}

func TestParseFloatDef(t *testing.T) {
	tests := []struct {
		in  string
		def float64
		out float64
	}{
		{"8.2", 0, 8.2},
		{"abc", 5.0, 5.0},
		{"", 3.0, 3.0},
		{"  10.5  ", 0, 10.5},
		{"0", 1.0, 0},
		{"-3.7", 0, -3.7},
	}
	for _, tt := range tests {
		v := parseFloatDef(tt.in, tt.def)
		if v != tt.out {
			t.Errorf("parseFloatDef(%q, %v) = %v; want %v", tt.in, tt.def, v, tt.out)
		}
	}
}

func TestParseIntDef(t *testing.T) {
	tests := []struct {
		in  string
		def int
		out int
	}{
		{"12", 0, 12},
		{"abc", 30, 30},
		{"  8  ", 0, 8},
		{"0", 5, 0},
	}
	for _, tt := range tests {
		v := parseIntDef(tt.in, tt.def)
		if v != tt.out {
			t.Errorf("parseIntDef(%q, %v) = %v; want %v", tt.in, tt.def, v, tt.out)
		}
	}
}

func TestCalcFontSize(t *testing.T) {
	tests := []struct {
		nome        string
		comprimento float64
		largura     float64
		raioTubo    float64
	}{
		{"JOÃO SILVA", 100, 0.7, 6.1},
		{"MARIA CLARA", 80, 0.5, 5.0},
		{"A", 50, 1.0, 10.0},
	}
	for _, tt := range tests {
		fs := calcFontSize(tt.nome, tt.comprimento, tt.largura, tt.raioTubo)
		if fs <= 0 {
			t.Errorf("calcFontSize(%q, %v, %v, %v) = %v; want > 0",
				tt.nome, tt.comprimento, tt.largura, tt.raioTubo, fs)
		}
		if math.IsNaN(fs) || math.IsInf(fs, 0) {
			t.Errorf("calcFontSize returned NaN/Inf: %v", fs)
		}
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
	checks := []string{
		"$fn = 12", "raio_furo", "raio_tubo", "raio_externo",
		"comprimento_tubo", "tamanho_fonte", "cylindrical_extrude",
		"cylinder(h = comprimento_tubo", "difference()",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("SCAD content missing: %s", c)
		}
	}
}

func TestGenerateScadContentSingleName(t *testing.T) {
	cfg := Config{
		Nome:            "MARIA",
		DiametroFuro:    6.0,
		EspessuraParede: 1.5,
		EspessuraRelevo: 2.0,
		ComprimentoTubo: 80,
		LarguraTexto:    0.6,
		Fn:              20,
	}
	content := generateScadContent(cfg)
	if !strings.Contains(content, "MARIA") {
		t.Error("Single name missing from SCAD")
	}
	if strings.Contains(content, "Lado2") {
		t.Error("Lado2 should be empty for single name")
	}
}

func TestFindOpenSCAD(t *testing.T) {
	path := findOpenSCAD()
	if path == "" {
		t.Error("OpenSCAD not found (expected nightly)")
	}
}

func TestExportSTL(t *testing.T) {
	openscadPath := findOpenSCAD()
	if openscadPath == "" {
		t.Skip("OpenSCAD not installed")
	}

	dir := t.TempDir()
	scadPath := dir + "/test.scad"
	stlPath := dir + "/test.stl"

	scad := `include <BOSL2/std.scad>
$fn = 12;
raio_furo = 4.1; raio_tubo = 6.1; raio_externo = 8.6;
comprimento_tubo = 100; tamanho_fonte = 15.0;
module texto_curvo(txt, angulo) { rotate(angulo) cylindrical_extrude(ir = raio_tubo - 0.1, or = raio_externo) { rotate(-90) text(txt, size = tamanho_fonte, font = "Arial:style=Bold", halign = "center", valign = "center"); } }
rotate([0, 90, 0]) difference() { union() { cylinder(h = 100, r = raio_tubo, center = true); texto_curvo("MARIA", 0); texto_curvo("CLARA", 180); } cylinder(h = 102, r = raio_furo, center = true); }`

	if err := os.WriteFile(scadPath, []byte(scad), 0644); err != nil {
		t.Fatal(err)
	}

	if err := exportSTL(openscadPath, scadPath, stlPath, context.Background()); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(stlPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("STL file is empty")
	}
}

func TestCameraThreadSafety(t *testing.T) {
	c := Camera{
		yaw:   -30 * math.Pi / 180,
		pitch: 25 * math.Pi / 180,
		dist:  200,
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.mu.Lock()
			c.yaw += 0.1
			c.pitch -= 0.05
			c.mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			c.mu.Lock()
			_ = c.yaw
			_ = c.pitch
			_ = c.dist
			c.mu.Unlock()
		}()
	}
	wg.Wait()
}

func TestCameraAngles(t *testing.T) {
	c := Camera{
		yaw:   -30 * math.Pi / 180,
		pitch: 25 * math.Pi / 180,
		dist:  200,
	}
	c.mu.Lock()
	y, p, d := c.yaw, c.pitch, c.dist
	c.mu.Unlock()
	if math.Abs(y+30*math.Pi/180) > 0.001 {
		t.Errorf("yaw = %v, want -30°", y)
	}
	if math.Abs(p-25*math.Pi/180) > 0.001 {
		t.Errorf("pitch = %v, want 25°", p)
	}
	if d != 200 {
		t.Errorf("dist = %v, want 200", d)
	}
}
