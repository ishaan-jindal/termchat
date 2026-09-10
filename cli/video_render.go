package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strconv"
	"strings"
)

const videoHalfBlock = "▀"

// videoJPEGQuality balances frame size against block fidelity for the
// transmit-side pixelated encode.
const videoJPEGQuality = 75

// maxVideoTiles caps how many streams one video panel shows at once.
const maxVideoTiles = 4

// sidebarTileVidH is the video line count inside each bordered sidebar tile.
const sidebarTileVidH = 4

// videoTile is one stream ready for the grid renderer.
type videoTile struct {
	nick     string
	pix      []byte // row-major RGB triples
	w        int
	h        int
	streamID uint32 // sender stream ID; zero for the local preview
	self     bool
}

// decodeVideoFrame decodes one JPEG frame into row-major RGB triples.
func decodeVideoFrame(frame []byte) ([]byte, int, int, error) {
	img, _, err := image.Decode(bytes.NewReader(frame))
	if err != nil {
		return nil, 0, 0, err
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w < 1 || h < 1 {
		return nil, 0, 0, fmt.Errorf("video frame has no pixels")
	}

	pix := make([]byte, w*h*3)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			i := (y*w + x) * 3
			pix[i] = byte(r >> 8)
			pix[i+1] = byte(g >> 8)
			pix[i+2] = byte(b >> 8)
		}
	}

	return pix, w, h, nil
}

// encodeVideoFrame encodes row-major RGB triples as one baseline JPEG.
func encodeVideoFrame(pix []byte, w, h int) ([]byte, error) {
	if w < 1 || h < 1 || len(pix) < w*h*3 {
		return nil, fmt.Errorf("video frame has no pixels")
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 3
			img.SetNRGBA(x, y, color.NRGBA{R: pix[i], G: pix[i+1], B: pix[i+2], A: 255})
		}
	}

	var buf bytes.Buffer

	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: videoJPEGQuality})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// renderVideoFrame scales RGB pixels to a cols by rows cell grid and renders
// one ANSI string per row; two source pixel rows share one half-block cell.
func renderVideoFrame(pix []byte, srcW, srcH, cols, rows int) []string {
	if cols < 1 || rows < 1 || srcW < 1 || srcH < 1 {
		return nil
	}

	if len(pix) < srcW*srcH*3 {
		return nil
	}

	scaled := scaleRGB(pix, srcW, srcH, cols, rows*2)

	return renderColorLines(scaled, cols, rows)
}

// scaleRGB area-averages src down (or up) to dstW by dstH; empty source
// ranges from upscaling reuse the nearest pixel instead of dividing by zero.
func scaleRGB(pix []byte, srcW, srcH, dstW, dstH int) []byte {
	out := make([]byte, dstW*dstH*3)

	for dy := 0; dy < dstH; dy++ {
		y0 := dy * srcH / dstH
		y1 := (dy + 1) * srcH / dstH

		if y1 <= y0 {
			y1 = y0 + 1
		}

		if y1 > srcH {
			y1 = srcH
		}

		for dx := 0; dx < dstW; dx++ {
			x0 := dx * srcW / dstW
			x1 := (dx + 1) * srcW / dstW

			if x1 <= x0 {
				x1 = x0 + 1
			}

			if x1 > srcW {
				x1 = srcW
			}

			var rs, gs, bs, n int

			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					i := (y*srcW + x) * 3
					rs += int(pix[i])
					gs += int(pix[i+1])
					bs += int(pix[i+2])
					n++
				}
			}

			o := (dy*dstW + dx) * 3
			out[o] = byte(rs / n)
			out[o+1] = byte(gs / n)
			out[o+2] = byte(bs / n)
		}
	}

	return out
}

func renderColorLines(pix []byte, cols, rows int) []string {
	lines := make([]string, 0, rows)

	var b strings.Builder

	for y := 0; y < rows; y++ {
		b.Reset()

		var lastF, lastB uint32 = ^uint32(0), ^uint32(0)

		top := y * 2 * cols
		bot := top + cols

		for x := 0; x < cols; x++ {
			ti := (top + x) * 3
			bi := (bot + x) * 3

			f := uint32(pix[ti])<<16 | uint32(pix[ti+1])<<8 | uint32(pix[ti+2])
			v := uint32(pix[bi])<<16 | uint32(pix[bi+1])<<8 | uint32(pix[bi+2])

			if f != lastF || v != lastB {
				b.WriteString("\x1b[38;2;")
				b.WriteString(strconv.Itoa(int(pix[ti])))
				b.WriteByte(';')
				b.WriteString(strconv.Itoa(int(pix[ti+1])))
				b.WriteByte(';')
				b.WriteString(strconv.Itoa(int(pix[ti+2])))
				b.WriteString(";48;2;")
				b.WriteString(strconv.Itoa(int(pix[bi])))
				b.WriteByte(';')
				b.WriteString(strconv.Itoa(int(pix[bi+1])))
				b.WriteByte(';')
				b.WriteString(strconv.Itoa(int(pix[bi+2])))
				b.WriteByte('m')
				lastF, lastB = f, v
			}

			b.WriteString(videoHalfBlock)
		}

		b.WriteString("\x1b[0m")
		lines = append(lines, b.String())
	}

	return lines
}

// layoutVideoGrid picks tile columns and rows for n streams.
func layoutVideoGrid(n int) (tcols, trows int) {
	switch {
	case n <= 1:
		return 1, 1
	case n == 2:
		return 2, 1
	default:
		return 2, 2
	}
}

// pixelateBuffer block-averages pix into chunky blocks and expands back to
// full size with nearest neighbor; depth 0 or 1 returns pix unchanged.
func pixelateBuffer(pix []byte, w, h, depth int) []byte {
	if depth <= 1 {
		return pix
	}

	dw := max((w+depth-1)/depth, 1)
	dh := max((h+depth-1)/depth, 1)

	down := scaleRGB(pix, w, h, dw, dh)
	out := make([]byte, w*h*3)

	for y := 0; y < h; y++ {
		sy := y * dh / h

		for x := 0; x < w; x++ {
			sx := x * dw / w
			si := (sy*dw + sx) * 3
			oi := (y*w + x) * 3
			out[oi], out[oi+1], out[oi+2] = down[si], down[si+1], down[si+2]
		}
	}

	return out
}

// renderVideoTiles renders up to maxVideoTiles streams into a cols-wide,
// rows-high block: one caption line plus video lines per tile, joined
// horizontally and padded with blanks.
func renderVideoTiles(tiles []videoTile, cols, rows int) []string {
	if cols < 1 || rows < 1 {
		return nil
	}

	if len(tiles) > maxVideoTiles {
		tiles = tiles[:maxVideoTiles]
	}

	if len(tiles) == 0 {
		return blankLines(cols, rows)
	}

	tcols, trows := layoutVideoGrid(len(tiles))
	tileW := cols / tcols
	tileH := rows / trows

	if tileW < 4 || tileH < 2 {
		return blankLines(cols, rows)
	}

	tileLines := make([][]string, 0, len(tiles))

	for _, tile := range tiles {
		tileLines = append(tileLines, renderTile(tile, tileW, tileH))
	}

	for len(tileLines) < tcols*trows {
		tileLines = append(tileLines, blankLines(tileW, tileH))
	}

	out := make([]string, 0, rows)

	for r := 0; r < trows; r++ {
		for i := 0; i < tileH; i++ {
			var b strings.Builder

			for c := 0; c < tcols; c++ {
				b.WriteString(tileLines[r*tcols+c][i])
			}

			line := b.String()
			rest := cols - tcols*tileW

			if rest > 0 {
				line += strings.Repeat(" ", rest)
			}

			out = append(out, line)
		}
	}

	return out
}

// renderTile renders one stream: a nick caption and tileH-1 video lines.
// Pixels arrive already pixelated from the sender; rendering never alters
// the anonymity floor.
func renderTile(tile videoTile, tileW, tileH int) []string {
	out := make([]string, 0, tileH)
	out = append(out, tileCaption(tile.nick, tileW))

	vidH := tileH - 1

	if vidH < 1 || len(tile.pix) == 0 {
		return append(out, blankLines(tileW, vidH)...)
	}

	return append(out, renderVideoFrame(tile.pix, tile.w, tile.h, tileW, vidH)...)
}

// tileCaption truncates the nick to the tile width; the chat view paints the
// theme background over the full line width afterwards.
func tileCaption(nick string, tileW int) string {
	cap := " " + nick
	runes := []rune(cap)

	if len(runes) > tileW {
		runes = runes[:tileW]
	}

	for len(runes) < tileW {
		runes = append(runes, ' ')
	}

	return string(runes)
}

func blankLines(cols, rows int) []string {
	out := make([]string, 0, rows)

	for range rows {
		out = append(out, strings.Repeat(" ", cols))
	}

	return out
}
