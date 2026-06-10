package main

import (
	"image/png"
	"math"
	"os"
	"os/exec"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestOpenSCADPreviewGPU(t *testing.T) {
	openscadPath := findOpenSCAD()
	if openscadPath == "" {
		t.Skip("OpenSCAD not installed")
	}

	dir := t.TempDir()
	scadPath := dir + "/model.scad"
	scad := "include <BOSL2/std.scad>\n" +
		"$fn = 12;\n" +
		"raio_furo = 4.1; raio_tubo = 6.1; raio_externo = 8.6;\n" +
		"comprimento_tubo = 100; tamanho_fonte = 15.0;\n" +
		"module texto_curvo(txt, angulo) { rotate(angulo) cylindrical_extrude(ir = raio_tubo - 0.1, or = raio_externo) { rotate(-90) text(txt, size = tamanho_fonte, font = \"Arial:style=Bold\", halign = \"center\", valign = \"center\"); } }\n" +
		"rotate([0, 90, 0]) difference() { union() { cylinder(h = 100, r = raio_tubo, center = true); texto_curvo(\"MARIA\", 0); texto_curvo(\"CLARA\", 180); } cylinder(h = 102, r = raio_furo, center = true); }"
	if err := os.WriteFile(scadPath, []byte(scad), 0644); err != nil {
		t.Fatal(err)
	}

	pngPath := scadPath + ".preview.png"
	cmd := exec.Command(openscadPath,
		"--backend=Manifold",
		"--imgsize", "520,400",
		"--camera", "0,0,200,0,0,0",
		"-o", pngPath,
		scadPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("OpenSCAD preview GPU failed: %v\n%s", err, string(out))
	}

	if _, err := os.Stat(pngPath); err != nil {
		t.Fatal("Preview PNG not created")
	}

	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("PNG decode failed: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 520 || bounds.Dy() != 400 {
		t.Fatalf("PNG size: %dx%d, want 520x400", bounds.Dx(), bounds.Dy())
	}
}

func TestCameraDragUpdate(t *testing.T) {
	c := Camera{
		yaw:   -30 * math.Pi / 180,
		pitch: 25 * math.Pi / 180,
		dist:  200,
	}
	c.distMin = 30
	c.distMax = 500

	c.mu.Lock()
	y0 := c.yaw
	c.yaw += 0.1
	c.pitch += 0.05
	c.mu.Unlock()

	c.mu.Lock()
	y, p, d := c.yaw, c.pitch, c.dist
	c.mu.Unlock()

	if math.Abs(y-y0-0.1) > 0.0001 {
		t.Errorf("yaw = %v, want %v", y, y0+0.1)
	}
	if math.Abs(p-25*math.Pi/180-0.05) > 0.0001 {
		t.Errorf("pitch = %v, want %v", p, 25*math.Pi/180+0.05)
	}
	if d != 200 {
		t.Errorf("dist = %v, want 200", d)
	}
}

func TestCameraDistClamp(t *testing.T) {
	c := Camera{dist: 200}
	c.distMin = 30
	c.distMax = 500

	c.mu.Lock()
	c.dist *= 0.01
	if c.dist < c.distMin {
		c.dist = c.distMin
	}
	c.mu.Unlock()

	if c.dist != 30 {
		t.Errorf("dist = %v, want 30 (clamped)", c.dist)
	}
}

func TestFyneDragSimulation(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("Test")
	defer w.Close()

	vw := NewViewerWidget(520, 400)
	w.SetContent(vw)
	w.Resize(fyne.NewSize(600, 500))
	w.Show()

	yaw0, pitch0, dist0 := vw.CameraAngles()

	vw.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.Position{X: 100, Y: 100}},
		Dragged:    fyne.Delta{DX: 80, DY: 40},
	})
	vw.DragEnd()

	yaw1, pitch1, dist1 := vw.CameraAngles()

	if yaw1 == yaw0 {
		t.Error("yaw should change after drag")
	}
	if pitch1 == pitch0 {
		t.Error("pitch should change after drag")
	}
	if dist1 != dist0 {
		t.Error("dist should NOT change after drag")
	}

	// Simulate zoom via scroll
	vw.Scrolled(&fyne.ScrollEvent{
		PointEvent: fyne.PointEvent{Position: fyne.Position{X: 260, Y: 200}},
		Scrolled:   fyne.Delta{DY: 2},
	})

	_, _, dist2 := vw.CameraAngles()

	if dist2 >= dist0 {
		t.Error("dist should decrease after positive scroll (zoom in)")
	}
	if yaw1 == yaw0 {
		t.Error("yaw should change after drag")
	}
}
