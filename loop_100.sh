#!/bin/bash
# Loop de iteração: build → test → e2e → report
PASS=0
FAIL=0
OUTFILE="/tmp/opencode_loop_100.log"

echo "=== LOOP 100 ITERAÇÕES ===" | tee "$OUTFILE"
echo "Início: $(date)" | tee -a "$OUTFILE"
echo "" | tee -a "$OUTFILE"

for i in $(seq 1 100); do
    printf "Iteração %3d/100: " $i | tee -a "$OUTFILE"
    
    # 1. Build
    if ! go build -ldflags="-s -w" -o marcador-tubular-gui . 2>/dev/null; then
        echo "❌ BUILD FAIL" | tee -a "$OUTFILE"
        FAIL=$((FAIL + 1))
        continue
    fi
    
    # 2. Binary size check
    SIZE=$(stat -f%z marcador-tubular-gui 2>/dev/null || stat -c%s marcador-tubular-gui)
    if [ "$SIZE" -gt $((25 * 1024 * 1024)) ]; then
        echo "❌ SIZE > 25MB ($((SIZE / 1048576))MB)" | tee -a "$OUTFILE"
        FAIL=$((FAIL + 1))
        continue
    fi
    
    # 3. go vet
    if ! go vet ./... 2>/dev/null; then
        echo "❌ VET FAIL" | tee -a "$OUTFILE"
        FAIL=$((FAIL + 1))
        continue
    fi
    
    # 4. Unit tests with race
    if ! go test -race -count=1 ./... 2>&1 | grep -q "^ok"; then
        echo "❌ TEST FAIL" | tee -a "$OUTFILE"
        FAIL=$((FAIL + 1))
        continue
    fi
    
    # 5. E2E tests
    if ! bash test_e2e.sh &>/dev/null; then
        echo "❌ E2E FAIL" | tee -a "$OUTFILE"
        FAIL=$((FAIL + 1))
        continue
    fi
    
    echo "✅" | tee -a "$OUTFILE"
    PASS=$((PASS + 1))
done

echo "" | tee -a "$OUTFILE"
echo "=== RESULTADO FINAL ===" | tee -a "$OUTFILE"
echo "Passou: $PASS / 100" | tee -a "$OUTFILE"
echo "Falhou: $FAIL / 100" | tee -a "$OUTFILE"
echo "Fim:    $(date)" | tee -a "$OUTFILE"

# Generate compact summary
echo "" | tee -a "$OUTFILE"
echo "=== LINHA DE STATUS ===" | tee -a "$OUTFILE"
if [ $FAIL -eq 0 ]; then
    echo "✅ 100/100 TODAS AS ITERAÇÕES PASSARAM" | tee -a "$OUTFILE"
else
    echo "⚠  $FAIL iterações falharam" | tee -a "$OUTFILE"
fi

exit $FAIL
