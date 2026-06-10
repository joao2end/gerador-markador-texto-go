package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
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
	"github.com/fogleman/fauxgl"
)

type AppState struct {
	config     Config
	outputDir  string
	tempDir    string
	lastScad   string
	log        *widget.Entry
	previewImg *canvas.Image
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

type Viewer3D struct {
	meshMu   sync.Mutex
	angleMu  sync.Mutex

	mesh      *fauxgl.Mesh
	center    fauxgl.Vector
	radius    float64
	ctx       *fauxgl.Context
	snapshot  *image.RGBA
	ready     bool

	yaw, pitch  float64
	dist        float64
	distMin     float64
	distMax     float64

	imgW, imgH  int
	lastMX, lastMY float64
	dragging    bool
	renderReq   chan struct{}
	onRender    func()

	// log
	logRenderMs func(ms int64)
}

type ViewerWidget struct {
	widget.BaseWidget
	viewer  *Viewer3D
	display *canvas.Image
}

func NewViewerWidget(w, h int) *ViewerWidget {
	vw := &ViewerWidget{
		viewer: &Viewer3D{
			imgW:      w,
			imgH:      h,
			yaw:       -30 * math.Pi / 180,
			pitch:     25 * math.Pi / 180,
			dist:      200,
			ctx:       fauxgl.NewContext(w, h),
			renderReq: make(chan struct{}, 1),
		},
		display: canvas.NewImageFromImage(nil),
	}
	vw.viewer.onRender = func() {
		fyne.Do(func() {
			vw.display.Image = vw.viewer.Image()
			canvas.Refresh(vw.display)
		})
	}
	vw.display.SetMinSize(fyne.NewSize(float32(w), float32(h)))
	vw.display.FillMode = canvas.ImageFillContain
	vw.ExtendBaseWidget(vw)
	go vw.viewer.renderLoop()
	return vw
}

func (vw *ViewerWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(vw.display)
}

func (vw *ViewerWidget) Dragged(ev *fyne.DragEvent) {
	v := vw.viewer
	ex := float64(ev.Position.X + ev.Dragged.DX)
	ey := float64(ev.Position.Y + ev.Dragged.DY)

	v.angleMu.Lock()
	if !v.dragging {
		v.lastMX = ex - float64(ev.Dragged.DX)
		v.lastMY = ey - float64(ev.Dragged.DY)
		v.dragging = true
	}
	dx := ex - v.lastMX
	dy := ey - v.lastMY
	v.lastMX = ex
	v.lastMY = ey
	v.yaw += dx * 0.008
	v.pitch += dy * 0.008
	if v.pitch > 89*math.Pi/180 {
		v.pitch = 89 * math.Pi / 180
	}
	if v.pitch < -89*math.Pi/180 {
		v.pitch = -89 * math.Pi / 180
	}
	v.angleMu.Unlock()

	v.requestRender()
}

func (vw *ViewerWidget) DragEnd() {
	vw.viewer.angleMu.Lock()
	vw.viewer.dragging = false
	vw.viewer.angleMu.Unlock()
}

func (vw *ViewerWidget) Scrolled(ev *fyne.ScrollEvent) {
	v := vw.viewer
	v.angleMu.Lock()
	v.dist *= (1 - float64(ev.Scrolled.DY)*0.08)
	if v.dist < v.distMin {
		v.dist = v.distMin
	}
	if v.dist > v.distMax {
		v.dist = v.distMax
	}
	v.angleMu.Unlock()
	v.requestRender()
}

func (v *Viewer3D) requestRender() {
	select {
	case v.renderReq <- struct{}{}:
	default:
	}
}

func (v *Viewer3D) renderLoop() {
	for range v.renderReq {
		v.render()
		if v.onRender != nil {
			v.onRender()
		}
	}
}

func (v *Viewer3D) SetMesh(mesh *fauxgl.Mesh) {
	v.meshMu.Lock()
	v.mesh = mesh
	if mesh != nil {
		bb := mesh.BoundingBox()
		v.center = bb.Center()
		v.radius = bb.Size().Length() / 2
		v.ready = true
	}
	v.meshMu.Unlock()

	if mesh != nil {
		v.angleMu.Lock()
		v.dist = v.radius * 3.5
		v.distMin = v.radius * 0.5
		v.distMax = v.radius * 20
		v.yaw = -30 * math.Pi / 180
		v.pitch = 25 * math.Pi / 180
		v.angleMu.Unlock()
	}
}

func (v *Viewer3D) render() {
	t0 := time.Now()

	v.angleMu.Lock()
	yaw, pitch, dist := v.yaw, v.pitch, v.dist
	v.angleMu.Unlock()

	v.meshMu.Lock()
	if !v.ready || v.mesh == nil {
		v.meshMu.Unlock()
		return
	}
	mesh := v.mesh
	cx, cy, cz := v.center.X, v.center.Y, v.center.Z
	center := v.center
	v.meshMu.Unlock()

	eyeX := cx + dist*math.Cos(pitch)*math.Sin(yaw)
	eyeY := cy + dist*math.Sin(pitch)
	eyeZ := cz + dist*math.Cos(pitch)*math.Cos(yaw)
	eye := fauxgl.Vector{X: eyeX, Y: eyeY, Z: eyeZ}
	up := fauxgl.Vector{X: 0, Y: 1, Z: 0}

	aspect := float64(v.imgW) / float64(v.imgH)
	mvp := fauxgl.Perspective(30, aspect, 0.1, dist*4).Mul(fauxgl.LookAt(eye, center, up))

	shader := fauxgl.NewPhongShader(mvp, fauxgl.Vector{X: 0.5, Y: 0.5, Z: 1}.Normalize(), eye)
	shader.ObjectColor = fauxgl.HexColor("#3B82A0")
	shader.AmbientColor = fauxgl.HexColor("#2A2A4A")
	shader.DiffuseColor = fauxgl.HexColor("#5B9EC4")
	shader.SpecularColor = fauxgl.HexColor("#FFFFFF")
	shader.SpecularPower = 60

	v.meshMu.Lock()
	v.ctx.Shader = shader
	v.ctx.ClearColor = fauxgl.HexColor("#F0F0F5")
	v.ctx.ClearColorBuffer()
	v.ctx.ClearDepthBuffer()
	v.ctx.ReadDepth = true
	v.ctx.WriteDepth = true
	v.ctx.Cull = fauxgl.CullBack
	v.ctx.FrontFace = fauxgl.FaceCCW
	v.ctx.DrawMesh(mesh)

	bounds := v.ctx.Image().Bounds()
	if v.snapshot == nil || !v.snapshot.Bounds().Eq(bounds) {
		v.snapshot = image.NewRGBA(bounds)
	}
	draw.Draw(v.snapshot, bounds, v.ctx.Image(), bounds.Min, draw.Src)
	v.meshMu.Unlock()

	if v.logRenderMs != nil {
		v.logRenderMs(time.Since(t0).Milliseconds())
	}
}

func (v *Viewer3D) Image() image.Image {
	return v.snapshot
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

func exportSTL(openscadPath, scadPath, stlPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, openscadPath, "--backend=Manifold", "-q", "-o", stlPath, scadPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("OpenSCAD excedeu o tempo limite (120s)")
		}
		return fmt.Errorf("%v\n%s", err, string(out))
	}
	return nil
}

var rendering atomic.Bool

func main() {
	a := app.New()
	w := a.NewWindow("Marcador Tubular - Preview 3D Interativo")
	w.Resize(fyne.NewSize(1050, 750))

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
	state.log = widget.NewMultiLineEntry()
	state.log.SetMinRowsVisible(3)
	state.log.Disable()

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
	scadPreview := widget.NewMultiLineEntry()
	scadPreview.SetMinRowsVisible(5)
	scadPreview.Wrapping = fyne.TextWrapBreak
	scadPreview.Disable()

	openscadPath := findOpenSCAD()
	scadPath := filepath.Join(tempDir, "model.scad")
	stlPath := filepath.Join(tempDir, "model.stl")

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

	genPreview := func() {
		if openscadPath == "" {
			dialog.ShowError(fmt.Errorf("OpenSCAD não encontrado"), w)
			return
		}

		if !rendering.CompareAndSwap(false, true) {
			state.log.SetText("Já existe uma operação em andamento. Aguarde...")
			return
		}

		previewBtn.Disable()
		state.config = buildConfig()

		content := generateScadContent(state.config)
		if err := os.WriteFile(scadPath, []byte(content), 0644); err != nil {
			state.log.SetText(fmt.Sprintf("Erro ao escrever SCAD: %v", err))
			previewBtn.Enable()
			rendering.Store(false)
			return
		}

		state.log.SetText("Exportando STL com Manifold...")
		tStartSTL := time.Now()

		go func() {
			tSTL := time.Now()
			if err := exportSTL(openscadPath, scadPath, stlPath); err != nil {
				fyne.Do(func() {
					state.log.SetText(fmt.Sprintf("Erro: %v", err))
					previewBtn.Enable()
					rendering.Store(false)
				})
				return
			}
			elapsedSTL := time.Since(tSTL)

			state.log.SetText(fmt.Sprintf("STL em %.1fs. Carregando malha...", elapsedSTL.Seconds()))
			tLoad := time.Now()
			mesh, err := fauxgl.LoadSTL(stlPath)
			if err != nil {
				fyne.Do(func() {
					state.log.SetText(fmt.Sprintf("Erro ao carregar STL: %v", err))
					previewBtn.Enable()
					rendering.Store(false)
				})
				return
			}
			elapsedLoad := time.Since(tLoad)

			vw.viewer.SetMesh(mesh)
			ntri := 0
			if mesh != nil {
				ntri = len(mesh.Triangles)
			}

			vw.viewer.logRenderMs = func(ms int64) {
				fyne.Do(func() {
					state.log.SetText(fmt.Sprintf(
						"✓ STL: %.1fs | Load: %.0fms | %d tri | Render: %dms\n  Arraste para orbitar | Scroll para zoom",
						elapsedSTL.Seconds(), float64(elapsedLoad.Microseconds())/1000, ntri, ms,
					))
				})
			}

			vw.viewer.render()
			img := vw.viewer.Image()

			fyne.Do(func() {
				vw.display.Image = img
				canvas.Refresh(vw.display)
				state.lastScad = scadPath
				previewBtn.Enable()
				rendering.Store(false)
			})
		}()
	}

	genBtn := widget.NewButtonWithIcon("Gerar .SCAD", theme.DocumentCreateIcon(), func() {
		state.config = buildConfig()
		sanitized := strings.ReplaceAll(state.config.Nome, " ", "_")
		out := filepath.Join(state.outputDir, fmt.Sprintf("marcador_%s.scad", sanitized))
		if err := os.WriteFile(out, []byte(generateScadContent(state.config)), 0644); err != nil {
			state.log.SetText(fmt.Sprintf("Erro ao salvar SCAD: %v", err))
			return
		}
		state.lastScad = out
		state.log.SetText(fmt.Sprintf("✓ SCAD salvo: %s", out))
	})

	var exportBtn *widget.Button
	exportBtn = widget.NewButtonWithIcon("Exportar STL (salvar)", theme.DownloadIcon(), func() {
		if openscadPath == "" {
			dialog.ShowError(fmt.Errorf("OpenSCAD não encontrado"), w)
			return
		}

		if !rendering.CompareAndSwap(false, true) {
			state.log.SetText("Já existe uma operação em andamento. Aguarde...")
			return
		}

		exportBtn.Disable()
		state.config = buildConfig()
		state.log.SetText("Exportando STL...")

		go func() {
			defer rendering.Store(false)

			tmp, err := os.CreateTemp(tempDir, "export-*.scad")
			if err != nil {
				fyne.Do(func() {
					state.log.SetText(fmt.Sprintf("Erro ao criar temp: %v", err))
					exportBtn.Enable()
				})
				return
			}
			tmpPath := tmp.Name()
			if _, err := tmp.Write([]byte(generateScadContent(state.config))); err != nil {
				tmp.Close()
				fyne.Do(func() {
					state.log.SetText(fmt.Sprintf("Erro ao escrever: %v", err))
					exportBtn.Enable()
				})
				return
			}
			tmp.Close()

			sanitized := strings.ReplaceAll(state.config.Nome, " ", "_")
			out := filepath.Join(state.outputDir, fmt.Sprintf("marcador_%s.stl", sanitized))

			if err := exportSTL(openscadPath, tmpPath, out); err != nil {
				fyne.Do(func() {
					state.log.SetText(fmt.Sprintf("Erro: %v", err))
					exportBtn.Enable()
				})
				return
			}

			info, err := os.Stat(out)
			sz := float64(0)
			if err == nil {
				sz = float64(info.Size()) / 1024
			}

			fyne.Do(func() {
				state.log.SetText(fmt.Sprintf("✓ STL salvo: %s (%.1f KB)", out, sz))
				exportBtn.Enable()
				dialog.ShowInformation("STL Exportado", fmt.Sprintf("%s\n%.1f KB", out, sz), w)
			})
		}()
	})

	openBtn := widget.NewButtonWithIcon("Abrir no OpenSCAD", theme.ComputerIcon(), func() {
		if state.lastScad == "" {
			state.lastScad = filepath.Join(state.outputDir, "marcador.scad")
		}
		if _, err := os.Stat(state.lastScad); os.IsNotExist(err) {
			state.lastScad = scadPath
		}
		if openscadPath != "" {
			exec.Command("open", "-a", openscadPath, state.lastScad).Start()
		}
	})

	folderBtn := widget.NewButton("📁", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err == nil && uri != nil {
				state.outputDir = uri.Path()
				outputEntry.SetText(state.outputDir)
			}
		}, w)
	})

	previewBtn = widget.NewButtonWithIcon("▶ Carregar Preview 3D", theme.VisibilityIcon(), genPreview)

	form := widget.NewForm(
		&widget.FormItem{Text: "Nome", Widget: nomeEntry, HintText: "Nome + Sobrenome"},
		&widget.FormItem{Text: "Diâmetro do Furo (mm)", Widget: diamFuroEntry, HintText: "8.2 (lápis)"},
		&widget.FormItem{Text: "Espessura da Parede (mm)", Widget: paredeEntry, HintText: "2.0"},
		&widget.FormItem{Text: "Altura do Relevo (mm)", Widget: relevoEntry, HintText: "2.5"},
		&widget.FormItem{Text: "Comprimento do Tubo (mm)", Widget: comprimentoEntry, HintText: "máx 100"},
		&widget.FormItem{Text: "Largura do Texto", Widget: container.NewBorder(nil, nil, widget.NewLabel("30%"), larguraLabel, larguraSlider)},
		&widget.FormItem{Text: "$fn (qualidade)", Widget: fnEntry, HintText: "20-30 rápido"},
	)

	outputRow := container.NewBorder(nil, nil, nil, folderBtn, outputEntry)

	leftPanel := container.NewVBox(
		widget.NewLabelWithStyle("Configuração", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		form,
		widget.NewLabelWithStyle("Diretório:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		outputRow,
		container.NewHBox(layout.NewSpacer(), genBtn, layout.NewSpacer()),
	)

	rightPanel := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Preview 3D Interativo", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Arraste para orbitar · Scroll para zoom", fyne.TextAlignCenter, fyne.TextStyle{}),
		),
		container.NewHBox(layout.NewSpacer(), previewBtn, layout.NewSpacer()),
		nil, nil,
		vw,
	)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.SetOffset(0.38)

	bottom := container.NewVBox(
		widget.NewLabelWithStyle("Código SCAD:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		scadPreview,
		widget.NewLabelWithStyle("Log:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		state.log,
		container.NewHBox(exportBtn, openBtn, layout.NewSpacer(), widget.NewButtonWithIcon("Sair", theme.CancelIcon(), func() { w.Close() })),
	)

	w.SetContent(container.NewBorder(nil, bottom, nil, nil, split))

	updateCode := func() {
		scadPreview.SetText(generateScadContent(buildConfig()))
	}

	larguraSlider.OnChanged = func(v float64) {
		larguraLabel.SetText(fmt.Sprintf("%.0f%%", v*100))
		updateCode()
	}
	onChange := func(_ string) { updateCode() }
	nomeEntry.OnChanged = onChange
	diamFuroEntry.OnChanged = onChange
	paredeEntry.OnChanged = onChange
	relevoEntry.OnChanged = onChange
	comprimentoEntry.OnChanged = onChange
	fnEntry.OnChanged = onChange

	w.SetOnClosed(func() { os.RemoveAll(tempDir) })
	updateCode()

	if openscadPath == "" {
		state.log.SetText("⚠ OpenSCAD não encontrado. Instale com: brew install openscad")
	} else {
		state.log.SetText("✓ OpenSCAD OK. Clique em \"Carregar Preview 3D\" para visualizar o modelo.")
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
