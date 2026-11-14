package ai

import (
	"bytes"
	"image"
	"image/png"
)

// tallAspect is how many times taller than wide a page must be to be split:
// a till receipt photographed whole. A model scales such a page down to fit
// its image size, until the receipt's text is too small to read.
const tallAspect = 2

// splitOverlap is the share of a tall page's height its two halves share,
// so a line cut in two by the split is whole in one of them.
const splitOverlap = 0.1

// splitNote tells the model how tall pages were sent.
const splitNote = "Pages much taller than wide, such as till receipts, are sent as two images: the top half, then the bottom half. The halves overlap a little, so a line in the overlap shows in both: count it once.\n"

// splitTall sends pages more than tallAspect times as tall as wide as two
// overlapping halves, top first, for models that read several images. Pages
// that cannot be decoded go as they are. It tells whether a page was split.
func splitTall(pages [][]byte) ([][]byte, bool) {
	out := make([][]byte, 0, len(pages))
	split := false
	for _, p := range pages {
		top, bottom, ok := halves(p)
		if !ok {
			out = append(out, p)
			continue
		}
		out = append(out, top, bottom)
		split = true
	}
	return out, split
}

// halves splits a tall PNG page into two overlapping halves.
func halves(page []byte) (top, bottom []byte, ok bool) {
	cfg, err := png.DecodeConfig(bytes.NewReader(page))
	if err != nil || cfg.Height <= tallAspect*cfg.Width {
		return nil, nil, false
	}
	img, err := png.Decode(bytes.NewReader(page))
	if err != nil {
		return nil, nil, false
	}
	sub, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return nil, nil, false
	}
	b := img.Bounds()
	mid, overlap := b.Min.Y+b.Dy()/2, int(float64(b.Dy())*splitOverlap/2)
	top, err1 := encodePNG(sub.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, mid+overlap)))
	bottom, err2 := encodePNG(sub.SubImage(image.Rect(b.Min.X, mid-overlap, b.Max.X, b.Max.Y)))
	if err1 != nil || err2 != nil {
		return nil, nil, false
	}
	return top, bottom, true
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
