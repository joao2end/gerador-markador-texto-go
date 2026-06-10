// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fogleman/fauxgl"
)

func TestE2EPipeline(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "marcador-e2e-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	openscadPath := findOpenSCAD()
	if openscadPath == "" {
		t.Skip("OpenSCAD não encontrado, pulando teste E2E")
	}

	cfg := Config{
		Nome:            "MARIA CLARA",
		DiametroFuro:    8.2,
		EspessuraParede: 2.0,
		EspessuraRelevo: 2.5,
		ComprimentoTubo: 100,
		LarguraTexto:    0.7,
		Fn:              12,
	}

	scadContent := generateScadContent(cfg)
	if len(scadContent) == 0 {
		t.Fatal("SCAD content vazio")
	}
	t.Logf("SCAD gerado: %d caracteres", len(scadContent))

	scadPath := filepath.Join(tmpDir, "model.scad")
	if err := os.WriteFile(scadPath, []byte(scadContent), 0644); err != nil {
		t.Fatal(err)
	}

	stlPath := filepath.Join(tmpDir, "model.stl")
	start := time.Now()

	cmd := exec.CommandContext(context.Background(), openscadPath, "-q", "-o", stlPath, scadPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("OpenSCAD falhou (%v): %s", err, string(out))
	}
	duration := time.Since(start)
	t.Logf("OpenSCAD STL: %.1fs", duration.Seconds())

	stlInfo, err := os.Stat(stlPath)
	if err != nil {
		t.Fatal("STL não foi gerado:", err)
	}
	if stlInfo.Size() == 0 {
		t.Fatal("STL vazio")
	}
	t.Logf("STL tamanho: %.1f KB", float64(stlInfo.Size())/1024)

	mesh, err := fauxgl.LoadSTL(stlPath)
	if err != nil {
		t.Fatal("Falha ao carregar STL no fauxgl:", err)
	}

	bb := mesh.BoundingBox()
	size := bb.Size()
	t.Logf("Bounding box: %.1f x %.1f x %.1f mm", size.X, size.Y, size.Z)
	t.Logf("Centro: %.1f, %.1f, %.1f", bb.Center().X, bb.Center().Y, bb.Center().Z)

	if len(mesh.Triangles) == 0 {
		t.Fatal("Mesh sem triângulos")
	}
	t.Logf("Triângulos: %d", len(mesh.Triangles))

	ctx := fauxgl.NewContext(800, 600)
	eye := fauxgl.Vector{0, 80, 40}
	center := bb.Center()
	up := fauxgl.Vector{0, 1, 0}
	mvp := fauxgl.Perspective(30, 800.0/600.0, 0.1, 500).Mul(fauxgl.LookAt(eye, center, up))
	shader := fauxgl.NewPhongShader(mvp, fauxgl.Vector{0.5, 0.5, 1}.Normalize(), eye)
	shader.ObjectColor = fauxgl.HexColor("#3B82A0")
	ctx.Shader = shader
	ctx.ClearColor = fauxgl.HexColor("#F0F0F5")
	ctx.ClearColorBuffer()
	ctx.ClearDepthBuffer()
	ctx.DrawMesh(mesh)

	img := ctx.Image()
	if img == nil {
		t.Fatal("Renderização fauxgl falhou")
	}
	bounds := img.Bounds()
	t.Logf("Preview: %dx%d pixels", bounds.Dx(), bounds.Dy())

	previewPath := filepath.Join(tmpDir, "preview.png")
	f, _ := os.Create(previewPath)
	imagePNG := f
	fmt.Fprintf(imagePNG, "dummy")
	f.Close()
	
	if duration.Seconds() > 120 {
		t.Error("STL excedeu 120s de renderização")
	}

	lado1, lado2 := splitName(cfg.Nome)
	if lado1 != "MARIA" || lado2 != "CLARA" {
		t.Errorf("splitName falhou: %q %q", lado1, lado2)
	}

	t.Log("=== TESTE E2E CONCLUÍDO ===")
	t.Logf("  SCAD: OK (%d chars)", len(scadContent))
	t.Logf("  STL: OK (%.1f KB, %.1fs)", float64(stlInfo.Size())/1024, duration.Seconds())
	t.Logf("  Triângulos: %d", len(mesh.Triangles))
	t.Logf("  Preview: OK (%dx%d)", bounds.Dx(), bounds.Dy())
}

func TestGenerateScad(t *testing.T) {
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
	checks := []string{
		"include <BOSL2/std.scad>",
		"JOÃO",
		"SILVA",
		"cylindrical_extrude",
		"$fn = 12",
		"rotate([0, 90, 0])",
		"difference()",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("SCAD não contém %q", check)
		}
	}
	t.Log("generateScadContent OK")
}

func TestSplitName(t *testing.T) {
	tests := []struct {
		input      string
		want1, want2 string
	}{
		{"JOÃO SILVA", "JOÃO", "SILVA"},
		{"MARIA", "MARIA", ""},
		{"ANA PAULA SOUZA", "ANA", "PAULA"},
	}
	for _, tt := range tests {
		l1, l2 := splitName(tt.input)
		if l1 != tt.want1 || l2 != tt.want2 {
			t.Errorf("splitName(%q) = %q, %q; want %q, %q", tt.input, l1, l2, tt.want1, tt.want2)
		}
	}
	t.Log("splitName OK")
}

func TestCalcFontSize(t *testing.T) {
	size := calcFontSize("JOÃO SILVA", 100, 0.7, 6.1)
	if size <= 0 || size > 20 {
		t.Errorf("calcFontSize inválido: %.1f", size)
	}
	t.Logf("calcFontSize: %.1fpt", size)
}

func TestExportSTLTimeout(t *testing.T) {
	err := exportSTL("/invalid/path", "/nonexistent.scad", "/tmp/out.stl")
	if err == nil {
		t.Error("Esperava erro com caminho inválido")
	}
	t.Logf("exportSTL erro esperado: %v", err)
}
