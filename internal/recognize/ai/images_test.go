package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// grayPNG is a w × h page, black at the top row and white at the bottom row.
func grayPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for x := range w {
		img.SetGray(x, h-1, color.Gray{Y: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func size(t *testing.T, p []byte) image.Point {
	t.Helper()
	cfg, err := png.DecodeConfig(bytes.NewReader(p))
	if err != nil {
		t.Fatal(err)
	}
	return image.Pt(cfg.Width, cfg.Height)
}

func TestSplitTallSplitsReceiptsIntoOverlappingHalves(t *testing.T) {
	receipt, page := grayPNG(t, 100, 300), grayPNG(t, 100, 141)
	out, split := splitTall([][]byte{receipt, page, pngPage})
	if !split || len(out) != 4 {
		t.Fatalf("split %v into %d images, want 4", split, len(out))
	}
	top, bottom := size(t, out[0]), size(t, out[1])
	if top != image.Pt(100, 165) || bottom != image.Pt(100, 165) {
		t.Errorf("halves %v and %v, want 100×165 each (half of 300 and 15 overlapping)", top, bottom)
	}
	img, _ := png.Decode(bytes.NewReader(out[1]))
	if g := color.GrayModel.Convert(img.At(0, img.Bounds().Max.Y-1)).(color.Gray); g.Y != 255 {
		t.Error("the bottom half does not end at the bottom of the page")
	}
	if !bytes.Equal(out[2], page) || !bytes.Equal(out[3], pngPage) {
		t.Error("pages that are not tall, or not PNG, must go as they are")
	}
	if _, split := splitTall([][]byte{page}); split {
		t.Error("an A4 page was split")
	}
}

func TestOllamaSendsTallPagesInHalves(t *testing.T) {
	srv, rec := serve(t, 200, `{"message":{"role":"assistant","content":"{}"},"done":true,"done_reason":"stop"}`)
	m, err := NewOllama(Config{Provider: ProviderOllama, URL: srv.URL, Model: "llava"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = m.Extract(context.Background(), "TOTAAL 36,30", [][]byte{grayPNG(t, 100, 300)})
	msgs := rec.body["messages"].([]any)
	user := msgs[1].(map[string]any)
	if images := user["images"].([]any); len(images) != 2 {
		t.Errorf("sent %d images, want the two halves", len(images))
	}
	if !strings.Contains(user["content"].(string), "top half") {
		t.Errorf("prompt does not explain the halves: %q", user["content"])
	}
}

func TestAnthropicSendsTallPagesInHalves(t *testing.T) {
	srv, rec := serve(t, 200, messageResponse("end_turn", fullAnswer))
	m := newTestAnthropic(t, srv, "")
	if _, err := m.Extract(context.Background(), "TOTAAL 36,30", [][]byte{grayPNG(t, 100, 300)}); err != nil {
		t.Fatal(err)
	}
	content := rec.body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 3 || !strings.Contains(content[2].(map[string]any)["text"].(string), "top half") {
		t.Errorf("content = %v, want two halves and the prompt", content)
	}
}

func TestNVIDIASendsOneWholePage(t *testing.T) {
	srv, rec := serve(t, 200, `{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}]}`)
	m, err := NewNVIDIA(Config{Provider: ProviderNVIDIA, URL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	receipt := grayPNG(t, 100, 300)
	_, _ = m.Extract(context.Background(), "TOTAAL 36,30", [][]byte{receipt})
	content := rec.body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	url := content[0].(map[string]any)["image_url"].(map[string]any)["url"].(string)
	if url != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(receipt) {
		t.Error("NVIDIA reads one image: the whole page must go")
	}
}

func TestPromptSaysTheImagesWinOverTheText(t *testing.T) {
	for _, p := range []string{systemPrompt, userPrompt("x")} {
		if !strings.Contains(p, "OCR") || !strings.Contains(p, "images") {
			t.Errorf("prompt does not say the OCR text may be wrong and the images decide: %q", p)
		}
	}
}
