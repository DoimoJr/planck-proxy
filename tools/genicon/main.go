// Tool: genera assets/planck.ico — un placeholder semplice con la "P"
// bianca su cerchio viola, in 6 risoluzioni (16/32/48/64/128/256 px).
//
// Uso:
//
//	go run ./tools/genicon
//
// Output: scrive assets/planck.ico nel CWD del repo. L'icona viene poi
// linkata nel binario tramite cmd/planck/rsrc_windows_amd64.syso (vedi
// build.bat / README per il setup di rsrc).
//
// Sostituibile: bastera' rimpiazzare il file .ico per upgrade visivo
// senza toccare questo tool. Non e' wired al build automatico (e' un
// run-once tool, l'.ico va in repo e cambia solo se vuoi).
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	sizes := []int{16, 32, 48, 64, 128, 256}
	var pngs [][]byte
	for _, s := range sizes {
		img := makeIcon(s)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			fmt.Fprintf(os.Stderr, "encode %dpx: %v\n", s, err)
			os.Exit(1)
		}
		pngs = append(pngs, buf.Bytes())
	}

	out := buildICO(pngs, sizes)

	dest := filepath.Join("assets", "planck.ico")
	if err := os.MkdirAll("assets", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir assets: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(dest, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", dest, err)
		os.Exit(1)
	}
	fmt.Printf("scritto %s (%d byte, %d risoluzioni)\n", dest, len(out), len(sizes))
}

// makeIcon disegna un cerchio viola pieno con anti-aliasing al bordo +
// una "P" bianca composta da asta verticale + anello (pancia) in alto.
// Lo sfondo e' trasparente cosi' l'icona si integra in qualunque tema.
func makeIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)

	// Cerchio di sfondo, viola Planck #8e44ad, con AA sul bordo.
	cx, cy := float64(size)/2.0-0.5, float64(size)/2.0-0.5
	r := float64(size)/2.0 - 1
	bg := color.RGBA{R: 0x8e, G: 0x44, B: 0xad, A: 0xff}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d := math.Sqrt(dx*dx + dy*dy)
			alpha := r - d
			if alpha <= 0 {
				continue
			}
			if alpha >= 1 {
				alpha = 1
			}
			a := uint8(alpha * 255)
			img.SetRGBA(x, y, color.RGBA{R: bg.R, G: bg.G, B: bg.B, A: a})
		}
	}

	// "P" bianca: asta + pancia. Coordinate in frazione di size.
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	// Asta verticale: colonna sinistra della P.
	astaX0 := frac(size, 0.30)
	astaX1 := frac(size, 0.43)
	astaY0 := frac(size, 0.25)
	astaY1 := frac(size, 0.78)
	for y := astaY0; y < astaY1; y++ {
		for x := astaX0; x < astaX1; x++ {
			img.SetRGBA(x, y, white)
		}
	}

	// Pancia: anello (cerchio cavo) in alto a destra dell'asta.
	pcx := float64(size) * 0.485
	pcy := float64(size) * 0.385
	pRout := float64(size) * 0.215
	pRin := float64(size) * 0.105
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-pcx, float64(y)+0.5-pcy
			d := math.Sqrt(dx*dx + dy*dy)
			if d > pRout {
				continue
			}
			if d < pRin {
				continue
			}
			// Edge AA su entrambi i bordi.
			outerA := math.Min(1, pRout-d)
			innerA := math.Min(1, d-pRin)
			alpha := math.Min(outerA, innerA)
			if alpha <= 0 {
				continue
			}
			a := uint8(alpha * 255)
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: a})
		}
	}

	return img
}

func frac(size int, f float64) int { return int(float64(size)*f + 0.5) }

// buildICO produce il container ICO multi-risoluzione che incapsula i
// PNG forniti. Formato: 6-byte header + N entry da 16 byte + payload PNG.
// Riferimento: https://en.wikipedia.org/wiki/ICO_(file_format)
func buildICO(pngs [][]byte, sizes []int) []byte {
	var buf bytes.Buffer
	// ICONDIR: reserved=0, type=1 (ico), count=N
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(pngs)))

	// Offset del primo blob = header (6) + N * entry (16).
	offset := 6 + 16*len(pngs)

	// ICONDIRENTRY: width(1) height(1) colors(1) reserved(1) planes(2)
	// bpp(2) bytesInRes(4) imageOffset(4)
	for i, p := range pngs {
		s := sizes[i]
		w := byte(s)
		h := byte(s)
		if s == 256 {
			// 256 si codifica come 0 (la dimensione massima).
			w = 0
			h = 0
		}
		buf.WriteByte(w)
		buf.WriteByte(h)
		buf.WriteByte(0) // palette (true color)
		buf.WriteByte(0) // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(&buf, binary.LittleEndian, uint32(len(p)))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(p)
	}

	// Payload: i PNG raw (Windows Vista+ riconosce PNG embedded).
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes()
}
