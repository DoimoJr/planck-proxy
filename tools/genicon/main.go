// Tool: genera assets/planck.ico — replica il brand-dot del logo Planck:
// quadrato nero con angoli arrotondati su sfondo trasparente, in 6
// risoluzioni (16/32/48/64/128/256 px).
//
// Uso:
//
//	go run ./tools/genicon
//
// Output: scrive assets/planck.ico + internal/web/public/favicon.ico.
// Per propagare l'icona al binario .exe rigenera anche il syso:
//
//	go run github.com/akavel/rsrc@latest -ico assets/planck.ico \
//	  -o cmd/planck/rsrc_windows_amd64.syso
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
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

	// Scriviamo in due posti: assets/planck.ico (master, usato da rsrc
	// per generare il syso) + internal/web/public/favicon.ico (servito
	// dal web server, usato da Edge in app mode come icona finestra).
	destinations := []string{
		filepath.Join("assets", "planck.ico"),
		filepath.Join("internal", "web", "public", "favicon.ico"),
	}
	for _, dest := range destinations {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", filepath.Dir(dest), err)
			os.Exit(1)
		}
		if err := os.WriteFile(dest, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", dest, err)
			os.Exit(1)
		}
		fmt.Printf("scritto %s (%d byte, %d risoluzioni)\n", dest, len(out), len(sizes))
	}
}

// makeIcon riproduce il brand-dot del logo Planck (.brand-dot in
// monitor.css): un quadrato nero pieno centrato, lato ~62% del canvas,
// border-radius 2/10 del lato. Sfondo trasparente.
func makeIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)

	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}

	inner := size * 62 / 100
	if inner < 4 {
		inner = 4
	}
	offset := (size - inner) / 2
	radius := inner * 2 / 10
	if radius < 1 {
		radius = 1
	}
	x0, y0 := offset, offset
	x1, y1 := offset+inner, offset+inner

	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			var cx, cy int
			cornered := false
			if x < x0+radius && y < y0+radius {
				cx, cy = x0+radius, y0+radius
				cornered = true
			} else if x >= x1-radius && y < y0+radius {
				cx, cy = x1-radius-1, y0+radius
				cornered = true
			} else if x < x0+radius && y >= y1-radius {
				cx, cy = x0+radius, y1-radius-1
				cornered = true
			} else if x >= x1-radius && y >= y1-radius {
				cx, cy = x1-radius-1, y1-radius-1
				cornered = true
			}
			if cornered {
				dx := x - cx
				dy := y - cy
				if dx*dx+dy*dy > radius*radius {
					continue
				}
			}
			img.SetRGBA(x, y, black)
		}
	}
	return img
}

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
