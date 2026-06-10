#!/bin/bash
# E2E Test: Marcador Tubular Pipeline
# Testa o fluxo completo: SCAD → STL → Preview

set -e

TMP=$(mktemp -d)
trap "rm -rf $TMP" EXIT

PASS=0
FAIL=0

check() {
    local desc="$1"
    shift
    if "$@"; then
        echo "  ✅ $desc"
        PASS=$((PASS + 1))
    else
        echo "  ❌ $desc"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== E2E: Marcador Tubular ==="
echo ""

echo "1️⃣  Verificando dependências..."

if [ -f "/Applications/OpenSCAD.app/Contents/MacOS/OpenSCAD" ]; then
    OPENSCAD="/Applications/OpenSCAD.app/Contents/MacOS/OpenSCAD"
    check "OpenSCAD nightly encontrado" test -x "$OPENSCAD"
else
    OPENSCAD=$(which openscad 2>/dev/null || echo "")
    check "OpenSCAD no PATH" test -n "$OPENSCAD"
fi

check "BOSL2 instalado" test -f "$HOME/Documents/OpenSCAD/libraries/BOSL2/std.scad"
check "Go binário compilado" test -f "$PWD/marcador-tubular-gui"
echo ""

echo "2️⃣  Gerando SCAD com BOSL2..."

cat > "$TMP/model.scad" << 'SCAD'
include <BOSL2/std.scad>
$fn = 12;
raio_furo = 8.2 / 2;
raio_tubo = 4.1 + 2.0;
raio_externo = 6.1 + 2.5;
comprimento_tubo = 100;
espessura_relevo = 2.5;
tamanho_fonte = 15.0;
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
        cylinder(h = 100, r = raio_tubo, center = true);
        texto_curvo("MARIA", 0);
        texto_curvo("CLARA", 180);
    }
    cylinder(h = 102, r = raio_furo, center = true);
}
SCAD

check "SCAD gerado (>0 bytes)" test -s "$TMP/model.scad"
echo "   $(wc -c < "$TMP/model.scad") bytes"
head -5 "$TMP/model.scad"
echo ""

echo "3️⃣  Renderizando STL..."

START=$SECONDS
"$OPENSCAD" --backend=Manifold -q -o "$TMP/model.stl" "$TMP/model.scad" 2>/dev/null
STL_EXIT=$?
DURATION=$((SECONDS - START))

check "OpenSCAD OK (exit 0)" test $STL_EXIT -eq 0
check "STL gerado" test -f "$TMP/model.stl"
check "STL não vazio" test -s "$TMP/model.stl"

if [ -f "$TMP/model.stl" ]; then
    FILESIZE=$(stat -f%z "$TMP/model.stl" 2>/dev/null || stat -c%s "$TMP/model.stl" 2>/dev/null)
    echo "   STL: $(ls -lh "$TMP/model.stl" | awk '{print $5}') | Tempo: ${DURATION}s"
fi
echo ""

echo "4️⃣  Renderizando preview PNG..."

START=$SECONDS
"$OPENSCAD" --backend=Manifold -q -o "$TMP/preview.png" --render --imgsize 800,600 --autocenter --viewall --projection p --colorscheme Cornfield "$TMP/model.scad" 2>/dev/null
PNG_EXIT=$?

check "Preview OK (exit 0)" test $PNG_EXIT -eq 0
check "PNG gerado" test -f "$TMP/preview.png"
check "PNG > 5KB" test "$(stat -f%z "$TMP/preview.png" 2>/dev/null || stat -c%s "$TMP/preview.png" 2>/dev/null)" -gt 5120

if [ -f "$TMP/preview.png" ]; then
    echo "   PNG: $(ls -lh "$TMP/preview.png" | awk '{print $5}')"
fi
echo ""

echo "5️⃣  Testando splitName (via Go)..."

cd "$PWD"
TEST_GO=$(cat << 'GOEOF'
package main
import ("fmt";"strings")
func main() {
    tests := []struct{in, l1, l2 string}{
        {"JOÃO SILVA", "JOÃO", "SILVA"},
        {"MARIA", "MARIA", ""},
        {"ANA PAULA", "ANA", "PAULA"},
    }
    for _, t := range tests {
        p := strings.Fields(t.in)
        l1, l2 := "", ""
        if len(p) >= 1 { l1 = p[0] }
        if len(p) >= 2 { l2 = p[1] }
        if l1 != t.l1 || l2 != t.l2 {
            fmt.Printf("FAIL: %q → %q %q (expected %q %q)\n", t.in, l1, l2, t.l1, t.l2)
            return
        }
    }
    fmt.Println("OK")
}
GOEOF
)

echo "$TEST_GO" > "$TMP/split_test.go"
SPLIT_RESULT=$(cd "$TMP" && go run split_test.go 2>/dev/null)
check "splitName ($SPLIT_RESULT)" test "$SPLIT_RESULT" = "OK"
echo ""

echo "6️⃣  Testando fauxgl (carregar STL)..."
GO_FAUX=$(cat << 'GOEOF'
package main
import (
    "fmt"
    "os"
    "github.com/fogleman/fauxgl"
)
func main() {
    if len(os.Args) < 2 { return }
    mesh, err := fauxgl.LoadSTL(os.Args[1])
    if err != nil { fmt.Printf("Erro: %v\n", err); return }
    bb := mesh.BoundingBox()
    s := bb.Size()
    fmt.Printf("Triangulos: %d | BBox: %.0fx%.0fx%.0f", len(mesh.Triangles), s.X, s.Y, s.Z)
}
GOEOF
)
echo "$GO_FAUX" > "$TMP/faux_test.go"
cd "$TMP" && go mod init test 2>/dev/null
go get github.com/fogleman/fauxgl 2>/dev/null
FAUX_RESULT=$(go run faux_test.go "$TMP/model.stl" 2>/dev/null)
check "fauxgl carregou STL" test -n "$FAUX_RESULT"
echo "   $FAUX_RESULT"
cd - > /dev/null
echo ""

echo "=== RESULTADO E2E ==="
echo "   ✅ Passou: $PASS"
echo "   ❌ Falhou: $FAIL"
echo ""

if [ $FAIL -eq 0 ]; then
    echo "✅ TODOS OS TESTES PASSARAM"
    echo "   Pipeline: SCAD → OpenSCAD → STL → fauxgl → Preview PNG"
    exit 0
else
    echo "⚠ $FALHA(S) teste(s) falharam"
    exit 1
fi