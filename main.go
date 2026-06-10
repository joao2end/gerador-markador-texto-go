package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type shadcnTheme struct{}

func (shadcnTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{0xF8, 0xFA, 0xFC, 0xFF}
	case theme.ColorNameForeground:
		return color.NRGBA{0x0F, 0x17, 0x2A, 0xFF}
	case theme.ColorNamePrimary:
		return color.NRGBA{0x0F, 0x17, 0x2A, 0xFF}
	case theme.ColorNameHover:
		return color.NRGBA{0xF1, 0xF5, 0xF9, 0xFF}
	case theme.ColorNamePressed:
		return color.NRGBA{0xE2, 0xE8, 0xF0, 0xFF}
	case theme.ColorNameDisabled:
		return color.NRGBA{0xE2, 0xE8, 0xF0, 0xFF}
	case theme.ColorNameInputBackground:
		return color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	case theme.ColorNameInputBorder:
		return color.NRGBA{0xE2, 0xE8, 0xF0, 0xFF}
	case theme.ColorNameScrollBar:
		return color.NRGBA{0xCB, 0xD5, 0xE1, 0xFF}
	case theme.ColorNameShadow:
		return color.NRGBA{0x0F, 0x17, 0x2A, 0x08}
	case theme.ColorNameSelection:
		return color.NRGBA{0x0F, 0x17, 0x2A, 0x14}
	case theme.ColorNameSuccess:
		return color.NRGBA{0x16, 0xA3, 0x4A, 0xFF}
	case theme.ColorNameWarning:
		return color.NRGBA{0xD9, 0x77, 0x06, 0xFF}
	case theme.ColorNameError:
		return color.NRGBA{0xDC, 0x26, 0x26, 0xFF}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (shadcnTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (shadcnTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (shadcnTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 12
	case theme.SizeNameInlineIcon:
		return 18
	case theme.SizeNameScrollBar:
		return 8
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameCaptionText:
		return 12
	}
	return theme.DefaultTheme().Size(name)
}

type AppState struct {
	config    Config
	outputDir string
	tempDir   string
	status    *widget.Label
	cancelSTL context.CancelFunc
}

type Config struct {
	Nome            string
	DiametroFuro    float64
	EspessuraParede float64
	EspessuraRelevo float64
	ComprimentoTubo float64
	LarguraTexto    float64
	Fn              int
}

type Camera struct {
	mu               sync.Mutex
	yaw, pitch, dist float64
	dragging         bool
	lastMX, lastMY   float64
	distMin, distMax float64
}

type ViewerWidget struct {
	widget.BaseWidget
	display      *canvas.Image
	imgW, imgH   int
	cam          Camera
	scrollTimer  *time.Timer
	renderFunc   func()
	prevRender   time.Time
	fullW, fullH int
}

func NewViewerWidget(w, h int) *ViewerWidget {
	vw := &ViewerWidget{
		imgW:  w,
		imgH:  h,
		fullW: w,
		fullH: h,
		display: canvas.NewImageFromImage(nil),
		cam: Camera{
			yaw:   -30 * math.Pi / 180,
			pitch: 25 * math.Pi / 180,
			dist:  200,
		},
	}
	vw.display.SetMinSize(fyne.NewSize(float32(w), float32(h)))
	vw.display.FillMode = canvas.ImageFillContain
	vw.ExtendBaseWidget(vw)
	return vw
}

func (vw *ViewerWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(vw.display)
}

func (vw *ViewerWidget) Dragged(ev *fyne.DragEvent) {
	c := &vw.cam
	ex := float64(ev.Position.X + ev.Dragged.DX)
	ey := float64(ev.Position.Y + ev.Dragged.DY)

	c.mu.Lock()
	if !c.dragging {
		c.lastMX = ex - float64(ev.Dragged.DX)
		c.lastMY = ey - float64(ev.Dragged.DY)
		c.dragging = true
	}
	dx := ex - c.lastMX
	dy := ey - c.lastMY
	c.lastMX = ex
	c.lastMY = ey
	c.yaw -= dx * 0.008
	c.pitch -= dy * 0.008
	if c.pitch > 89*math.Pi/180 {
		c.pitch = 89 * math.Pi / 180
	}
	if c.pitch < -89*math.Pi/180 {
		c.pitch = -89 * math.Pi / 180
	}
	c.mu.Unlock()

	vw.imgW = vw.fullW / 2
	vw.imgH = vw.fullH / 2

	if vw.renderFunc != nil && time.Since(vw.prevRender) >= 100*time.Millisecond {
		vw.prevRender = time.Now()
		vw.renderFunc()
	}
}

func (vw *ViewerWidget) DragEnd() {
	vw.cam.mu.Lock()
	vw.cam.dragging = false
	vw.cam.mu.Unlock()
	vw.imgW = vw.fullW
	vw.imgH = vw.fullH
	if vw.renderFunc != nil {
		vw.renderFunc()
	}
}

func (vw *ViewerWidget) Scrolled(ev *fyne.ScrollEvent) {
	c := &vw.cam
	c.mu.Lock()
	c.dist *= (1 - float64(ev.Scrolled.DY)*0.08)
	if c.dist < c.distMin {
		c.dist = c.distMin
	}
	if c.dist > c.distMax {
		c.dist = c.distMax
	}
	c.mu.Unlock()

	vw.imgW = vw.fullW / 2
	vw.imgH = vw.fullH / 2

	if vw.renderFunc != nil && time.Since(vw.prevRender) >= 100*time.Millisecond {
		vw.prevRender = time.Now()
		vw.renderFunc()
	}

	if vw.renderFunc != nil {
		if vw.scrollTimer != nil {
			vw.scrollTimer.Stop()
		}
		vw.scrollTimer = time.AfterFunc(200*time.Millisecond, func() {
			vw.imgW = vw.fullW
			vw.imgH = vw.fullH
			vw.renderFunc()
		})
	}
}

func (vw *ViewerWidget) CameraAngles() (yaw, pitch, dist float64) {
	vw.cam.mu.Lock()
	yaw, pitch, dist = vw.cam.yaw, vw.cam.pitch, vw.cam.dist
	vw.cam.mu.Unlock()
	return
}

func (vw *ViewerWidget) SetCameraAngles(yaw, pitch, dist float64) {
	vw.cam.mu.Lock()
	vw.cam.yaw = yaw
	vw.cam.pitch = pitch
	vw.cam.dist = dist
	vw.cam.mu.Unlock()
}

func (vw *ViewerWidget) SetCameraDistRange(min, max float64) {
	vw.cam.mu.Lock()
	vw.cam.distMin = min
	vw.cam.distMax = max
	if vw.cam.dist < min {
		vw.cam.dist = min
	}
	if vw.cam.dist > max {
		vw.cam.dist = max
	}
	vw.cam.mu.Unlock()
}

var scadTemplate = `include <BOSL2/std.scad>

$fn = {{.Fn}};

raio_furo = {{.DiametroFuro}} / 2;
raio_tubo = raio_furo + {{.EspessuraParede}};
raio_externo = raio_tubo + {{.EspessuraRelevo}};
comprimento_tubo = {{.ComprimentoTubo}};
espessura_relevo = {{.EspessuraRelevo}};
tamanho_fonte = {{.TamanhoFonte}};

module texto_curvo(txt, angulo) {
    rotate(angulo)
    cylindrical_extrude(ir = raio_tubo - 0.1, or = raio_externo) {
        rotate(-90)
        text(txt, size = tamanho_fonte, font = "Arial:style=Bold", halign = "center", valign = "center");
    }
}

rotate([0, 90, 0])
difference() {
    union() {
        cylinder(h = comprimento_tubo, r = raio_tubo, center = true);
        texto_curvo("{{.Lado1}}", 0);
        texto_curvo("{{.Lado2}}", 180);
    }
    cylinder(h = comprimento_tubo + 2, r = raio_furo, center = true);
}
`

type ScadParams struct {
	Lado1           string
	Lado2           string
	DiametroFuro    float64
	EspessuraParede float64
	EspessuraRelevo float64
	ComprimentoTubo float64
	LarguraTexto    float64
	Fn              int
	TamanhoFonte    float64
}

func calcFontSize(nome string, comprimento, larguraTexto, raioTubo float64) float64 {
	lado1, lado2 := splitName(nome)
	maxChars := math.Max(float64(len(lado1)), float64(len(lado2)))
	if maxChars < 1 {
		maxChars = 1
	}
	fromLength := larguraTexto * comprimento / (maxChars * 0.65)
	fromCircumference := math.Pi * raioTubo
	return math.Min(fromLength, fromCircumference)
}

func findOpenSCAD() string {
	paths := []string{
		"/Applications/OpenSCAD.app/Contents/MacOS/OpenSCAD",
		"/Applications/OpenSCAD-2021.01.app/Contents/MacOS/OpenSCAD",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("openscad"); err == nil {
		return path
	}
	return ""
}

func splitName(nome string) (lado1, lado2 string) {
	parts := strings.Fields(nome)
	if len(parts) >= 1 {
		lado1 = parts[0]
	}
	if len(parts) >= 2 {
		lado2 = parts[1]
	}
	return
}

func generateScadContent(cfg Config) string {
	lado1, lado2 := splitName(cfg.Nome)
	raioFuro := cfg.DiametroFuro / 2
	raioTubo := raioFuro + cfg.EspessuraParede
	tamanhoFonte := calcFontSize(cfg.Nome, cfg.ComprimentoTubo, cfg.LarguraTexto, raioTubo)
	params := ScadParams{
		Lado1:           lado1,
		Lado2:           lado2,
		DiametroFuro:    cfg.DiametroFuro,
		EspessuraParede: cfg.EspessuraParede,
		EspessuraRelevo: cfg.EspessuraRelevo,
		ComprimentoTubo: cfg.ComprimentoTubo,
		LarguraTexto:    cfg.LarguraTexto,
		Fn:              cfg.Fn,
		TamanhoFonte:    math.Round(tamanhoFonte*10) / 10,
	}
	replacer := strings.NewReplacer(
		"{{.Lado1}}", params.Lado1,
		"{{.Lado2}}", params.Lado2,
		"{{.DiametroFuro}}", fmt.Sprintf("%.1f", params.DiametroFuro),
		"{{.EspessuraParede}}", fmt.Sprintf("%.1f", params.EspessuraParede),
		"{{.EspessuraRelevo}}", fmt.Sprintf("%.1f", params.EspessuraRelevo),
		"{{.ComprimentoTubo}}", fmt.Sprintf("%.0f", params.ComprimentoTubo),
		"{{.LarguraTexto}}", fmt.Sprintf("%.2f", params.LarguraTexto),
		"{{.Fn}}", fmt.Sprintf("%d", params.Fn),
		"{{.TamanhoFonte}}", fmt.Sprintf("%.1f", params.TamanhoFonte),
	)
	return replacer.Replace(scadTemplate)
}

func exportSTL(openscadPath, scadPath, stlPath string, parentCtx context.Context) error {
	ctx, cancel := context.WithTimeout(parentCtx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, openscadPath, "--backend=Manifold", "-q", "-o", stlPath, scadPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("OpenSCAD excedeu o tempo limite (120s)")
		}
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("Exportação cancelada")
		}
		return fmt.Errorf("OpenSCAD: %v\n%s", err, string(out))
	}
	return nil
}

func genPreview(openscadPath, scadPath string, vw *ViewerWidget, status *widget.Label, onDone func()) {
	t0 := time.Now()

	yaw, pitch, dist := vw.CameraAngles()

	eyeX := dist * math.Cos(pitch) * math.Sin(yaw)
	eyeY := dist * math.Sin(pitch)
	eyeZ := dist * math.Cos(pitch) * math.Cos(yaw)

	pngPath := scadPath + ".preview.png"
	camera := fmt.Sprintf("%.1f,%.1f,%.1f,0,0,0", eyeX, eyeY, eyeZ)
	imgsz := fmt.Sprintf("%d,%d", vw.imgW, vw.imgH)

	cmd := exec.Command(openscadPath,
		"--backend=Manifold",
		"--imgsize", imgsz,
		"--camera", camera,
		"-o", pngPath,
		scadPath,
	)
	if err := cmd.Run(); err != nil {
		fyne.Do(func() {
			status.SetText(fmt.Sprintf("Erro no preview: %v", err))
		})
		return
	}

	f, err := os.Open(pngPath)
	if err != nil {
		fyne.Do(func() {
			status.SetText(fmt.Sprintf("Erro ao abrir preview: %v", err))
		})
		return
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		fyne.Do(func() {
			status.SetText(fmt.Sprintf("Erro ao decodificar PNG: %v", err))
		})
		return
	}

	elapsed := time.Since(t0)
	fyne.Do(func() {
		vw.display.Image = img
		canvas.Refresh(vw.display)
		status.SetText(fmt.Sprintf("Preview GPU em %.1fs", elapsed.Seconds()))
		if onDone != nil {
			onDone()
		}
	})
}

var rendering atomic.Bool
var renderQueued atomic.Bool

func main() {
	a := app.New()
	a.Settings().SetTheme(&shadcnTheme{})
	w := a.NewWindow("Marcador Tubular")
	w.Resize(fyne.NewSize(1100, 720))

	tempDir, _ := os.MkdirTemp("", "marcador-*")

	vw := NewViewerWidget(520, 400)

	cfgDefault := Config{
		Nome:            "JOÃO SILVA",
		DiametroFuro:    8.2,
		EspessuraParede: 2.0,
		EspessuraRelevo: 2.5,
		ComprimentoTubo: 100,
		LarguraTexto:    0.7,
		Fn:              12,
	}

	state := &AppState{
		config:    cfgDefault,
		outputDir: ".",
		tempDir:   tempDir,
	}
	state.status = widget.NewLabel("")

	nomeEntry := widget.NewEntry()
	nomeEntry.SetText(state.config.Nome)
	diamFuroEntry := widget.NewEntry()
	diamFuroEntry.SetText(fmt.Sprintf("%.1f", state.config.DiametroFuro))
	paredeEntry := widget.NewEntry()
	paredeEntry.SetText(fmt.Sprintf("%.1f", state.config.EspessuraParede))
	relevoEntry := widget.NewEntry()
	relevoEntry.SetText(fmt.Sprintf("%.1f", state.config.EspessuraRelevo))
	comprimentoEntry := widget.NewEntry()
	comprimentoEntry.SetText(fmt.Sprintf("%.0f", state.config.ComprimentoTubo))
	fnEntry := widget.NewEntry()
	fnEntry.SetText(fmt.Sprintf("%d", state.config.Fn))

	larguraSlider := widget.NewSlider(0.3, 1.0)
	larguraSlider.SetValue(state.config.LarguraTexto)
	larguraLabel := widget.NewLabel(fmt.Sprintf("%.0f%%", state.config.LarguraTexto*100))

	outputEntry := widget.NewEntry()
	outputEntry.SetText(state.outputDir)

	ctxSTL, cancelSTL := context.WithCancel(context.Background())
	state.cancelSTL = cancelSTL

	w.SetCloseIntercept(func() {
		state.cancelSTL()
		os.RemoveAll(tempDir)
		w.Close()
	})

	openscadPath := findOpenSCAD()
	scadPath := filepath.Join(tempDir, "model.scad")

	var previewBtn *widget.Button

	buildConfig := func() Config {
		return Config{
			Nome:            nomeEntry.Text,
			DiametroFuro:    parseFloatDef(diamFuroEntry.Text, 8.2),
			EspessuraParede: parseFloatDef(paredeEntry.Text, 2.0),
			EspessuraRelevo: parseFloatDef(relevoEntry.Text, 2.5),
			ComprimentoTubo: parseFloatDef(comprimentoEntry.Text, 100),
			LarguraTexto:    larguraSlider.Value,
			Fn:              parseIntDef(fnEntry.Text, 12),
		}
	}

	var queueRender func()
	queueRender = func() {
		if openscadPath == "" {
			return
		}
		if rendering.CompareAndSwap(false, true) {
			go genPreview(openscadPath, scadPath, vw, state.status, func() {
				rendering.Store(false)
				if renderQueued.CompareAndSwap(true, false) {
					queueRender()
				}
			})
		} else {
			renderQueued.Store(true)
		}
	}
	vw.renderFunc = queueRender

	requestPreview := func() {
		if openscadPath == "" {
			dialog.ShowError(fmt.Errorf("OpenSCAD não encontrado"), w)
			return
		}

		if !rendering.CompareAndSwap(false, true) {
			state.status.SetText("Renderizando... aguarde")
			return
		}

		previewBtn.Disable()
		state.config = buildConfig()

		content := generateScadContent(state.config)
		if err := os.WriteFile(scadPath, []byte(content), 0644); err != nil {
			state.status.SetText(fmt.Sprintf("Erro: %v", err))
			previewBtn.Enable()
			rendering.Store(false)
			return
		}

		vw.SetCameraDistRange(30, 500)

		state.status.SetText("Renderizando preview...")

		go genPreview(openscadPath, scadPath, vw, state.status, func() {
			fyne.Do(func() {
				previewBtn.Enable()
			})
			rendering.Store(false)
			if renderQueued.CompareAndSwap(true, false) {
				queueRender()
			}
		})
	}

	genBtn := &widget.Button{}
	genBtn = widget.NewButtonWithIcon("SCAD", theme.DocumentCreateIcon(), func() {
		state.config = buildConfig()
		sanitized := strings.ReplaceAll(state.config.Nome, " ", "_")
		out := filepath.Join(state.outputDir, fmt.Sprintf("marcador_%s.scad", sanitized))
		if err := os.WriteFile(out, []byte(generateScadContent(state.config)), 0644); err != nil {
			state.status.SetText(fmt.Sprintf("Erro: %v", err))
			return
		}
		state.status.SetText(fmt.Sprintf("SCAD salvo: %s", filepath.Base(out)))
	})

	var exportBtn *widget.Button
	exportBtn = widget.NewButtonWithIcon("STL", theme.DownloadIcon(), func() {
		if openscadPath == "" {
			dialog.ShowError(fmt.Errorf("OpenSCAD não encontrado"), w)
			return
		}

		if !rendering.CompareAndSwap(false, true) {
			state.status.SetText("Exportando... aguarde")
			return
		}

		exportBtn.Disable()
		state.config = buildConfig()
		state.status.SetText("Exportando STL...")

		go func() {
			tmp, err := os.CreateTemp(tempDir, "export-*.scad")
			if err != nil {
				fyne.Do(func() {
					state.status.SetText(fmt.Sprintf("Erro: %v", err))
					exportBtn.Enable()
					rendering.Store(false)
				})
				return
			}
			tmpPath := tmp.Name()
			if _, err := tmp.Write([]byte(generateScadContent(state.config))); err != nil {
				tmp.Close()
				fyne.Do(func() {
					state.status.SetText(fmt.Sprintf("Erro: %v", err))
					exportBtn.Enable()
					rendering.Store(false)
				})
				return
			}
			tmp.Close()

			sanitized := strings.ReplaceAll(state.config.Nome, " ", "_")
			out := filepath.Join(state.outputDir, fmt.Sprintf("marcador_%s.stl", sanitized))

			if err := exportSTL(openscadPath, tmpPath, out, ctxSTL); err != nil {
				fyne.Do(func() {
					state.status.SetText(fmt.Sprintf("Erro: %v", err))
					exportBtn.Enable()
					rendering.Store(false)
				})
				return
			}

			info, err := os.Stat(out)
			sz := float64(0)
			if err == nil {
				sz = float64(info.Size()) / 1024
			}

			fyne.Do(func() {
				state.status.SetText(fmt.Sprintf("STL exportado: %s (%.0f KB)", filepath.Base(out), sz))
				exportBtn.Enable()
				dialog.ShowInformation("STL Exportado", fmt.Sprintf("%s\n%.0f KB", out, sz), w)
				rendering.Store(false)
			})
		}()
	})

	folderBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err == nil && uri != nil {
				state.outputDir = uri.Path()
				outputEntry.SetText(state.outputDir)
			}
		}, w)
	})

	previewBtn = widget.NewButtonWithIcon("Carregar Preview 3D", theme.VisibilityIcon(), requestPreview)

	form := widget.NewForm(
		&widget.FormItem{Text: "Nome", Widget: nomeEntry, HintText: "Nome + Sobrenome"},
		&widget.FormItem{Text: "Furo (mm)", Widget: diamFuroEntry, HintText: "8.2"},
		&widget.FormItem{Text: "Parede (mm)", Widget: paredeEntry, HintText: "2.0"},
		&widget.FormItem{Text: "Relevo (mm)", Widget: relevoEntry, HintText: "2.5"},
		&widget.FormItem{Text: "Comprimento (mm)", Widget: comprimentoEntry, HintText: "máx 100"},
		&widget.FormItem{Text: "Largura texto", Widget: container.NewBorder(nil, nil, widget.NewLabel("30%"), larguraLabel, larguraSlider)},
		&widget.FormItem{Text: "Qualidade $fn", Widget: fnEntry, HintText: "20-30 rápido"},
	)

	outputRow := container.NewBorder(nil, nil, nil, folderBtn, outputEntry)

	formCard := container.NewVBox(
		widget.NewLabelWithStyle("Marcador Tubular", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.NewPadded(widget.NewSeparator()),
		form,
		container.NewPadded(widget.NewSeparator()),
		container.NewVBox(
			widget.NewLabelWithStyle("Diretório", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			outputRow,
		),
		container.NewPadded(layout.NewSpacer()),
		container.NewGridWithColumns(2, genBtn, exportBtn),
	)

	leftPanel := container.NewPadded(container.NewBorder(
		nil, nil, nil, nil,
		formCard,
	))

	makeViewBtn := func(label string, icon fyne.Resource, yaw, pitch, dist float64) *widget.Button {
		return widget.NewButtonWithIcon(label, icon, func() {
			vw.SetCameraAngles(yaw, pitch, dist)
			if vw.renderFunc != nil {
				vw.renderFunc()
			}
		})
	}

	viewToolbar := container.NewHBox(
		layout.NewSpacer(),
		makeViewBtn("", theme.NavigateBackIcon(), 0, 0, 200),
		makeViewBtn("", theme.ContentUndoIcon(), math.Pi, 0, 200),
		makeViewBtn("", theme.NavigateBackIcon(), -math.Pi/2, 0, 200),
		makeViewBtn("", theme.NavigateNextIcon(), math.Pi/2, 0, 200),
		makeViewBtn("", theme.MoveUpIcon(), 0, math.Pi/2, 200),
		makeViewBtn("", theme.ComputerIcon(), -30*math.Pi/180, 25*math.Pi/180, 200),
		layout.NewSpacer(),
	)

	rightPanel := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Preview 3D", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Arraste para orbitar · Scroll para zoom", fyne.TextAlignCenter, fyne.TextStyle{}),
		),
		container.NewVBox(
			viewToolbar,
			container.NewHBox(layout.NewSpacer(), previewBtn, layout.NewSpacer()),
			state.status,
		),
		nil, nil,
		vw,
	)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.SetOffset(0.35)

	w.SetContent(split)

	if openscadPath == "" {
		state.status.SetText("OpenSCAD não encontrado. Instale com: brew install openscad")
	} else {
		state.status.SetText("Clique em Carregar Preview 3D para visualizar")
	}

	w.ShowAndRun()
}

func parseFloatDef(s string, d float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return d
	}
	return v
}

func parseIntDef(s string, d int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return d
	}
	return v
}


