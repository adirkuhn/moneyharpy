package ocr_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/ocr"
)

func serve(t *testing.T, h http.HandlerFunc) ocr.Service {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return ocr.New(srv.URL + "/")
}

func TestReadSendsTheFileAndDecodesTheText(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/read" || string(body) != "%PDF-1.4" ||
			r.Header.Get("Content-Type") != "application/pdf" || r.Header.Get("X-Filename") != "invoice%202026.pdf" {
			t.Errorf("request = %s %s %q %v", r.Method, r.URL, body, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"Example Shop B.V.\fpage 2","method":"text","pages":2,"text_method":"rapidocr","embedded":[{"name":"factur-x.xml","content_type":"application/xml","data":"PEludm9pY2UvPg=="}],"alternatives":[{"method":"tesseract-psm6","page_mode":"6","text":"other reading"}]}`)
	})
	got, err := c.Read(context.Background(), ocr.File{Name: "invoice 2026.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Example Shop B.V.\fpage 2" || got.Method != "text" || got.Pages != 2 || len(got.Embedded) != 1 ||
		got.Embedded[0].Name != "factur-x.xml" || string(got.Embedded[0].Data) != "<Invoice/>" ||
		got.TextMethod != "rapidocr" ||
		len(got.Alternatives) != 1 || got.Alternatives[0] != (ocr.Reading{Method: "tesseract-psm6", Text: "other reading"}) {
		t.Errorf("read = %+v", got)
	}
}

func TestRenderReturnsThePageAndCount(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/render" || r.URL.Query().Get("page") != "2" {
			t.Errorf("request = %s", r.URL)
		}
		w.Header().Set("X-Page-Count", "3")
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "\x89PNG")
	})
	got, err := c.Render(context.Background(), ocr.File{Data: []byte("%PDF")}, 2)
	if err != nil || string(got.PNG) != "\x89PNG" || got.Count != 3 {
		t.Errorf("render = %+v, %v", got, err)
	}
}

func TestRenderAtAsksForTheSize(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/render" || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("max_side") != "2048" {
			t.Errorf("request = %s", r.URL)
		}
		w.Header().Set("X-Page-Count", "1")
		_, _ = io.WriteString(w, "\x89PNG")
	})
	got, err := c.RenderAt(context.Background(), ocr.File{Data: []byte("\xff\xd8\xff")}, 1, 2048)
	if err != nil || string(got.PNG) != "\x89PNG" || got.Count != 1 {
		t.Errorf("render = %+v, %v", got, err)
	}
	if _, err := c.RenderAt(context.Background(), ocr.File{}, 1, 0); err == nil {
		t.Error("a side of 0 pixels was asked for")
	}
}

func TestErrorsMapToTheServiceErrors(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusUnsupportedMediaType:  ocr.ErrUnsupported,
		http.StatusUnprocessableEntity:   ocr.ErrUnsupported,
		http.StatusRequestEntityTooLarge: ocr.ErrUnsupported,
		http.StatusNotFound:              ocr.ErrNoPage,
		http.StatusInternalServerError:   ocr.ErrUnavailable,
		http.StatusGatewayTimeout:        ocr.ErrUnavailable,
	} {
		c := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		if _, err := c.Render(context.Background(), ocr.File{Data: []byte("x")}, 1); !errors.Is(err, want) {
			t.Errorf("render %d = %v, want %v", status, err, want)
		}
		if status == http.StatusNotFound {
			continue
		}
		if _, err := c.Read(context.Background(), ocr.File{Data: []byte("x")}); !errors.Is(err, want) {
			t.Errorf("read %d = %v, want %v", status, err, want)
		}
	}
	if _, err := ocr.New("http://ocr.invalid:1").Render(context.Background(), ocr.File{}, 0); !errors.Is(err, ocr.ErrNoPage) {
		t.Errorf("page 0 = %v", err)
	}
}

func TestPing(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("ping = %v", err)
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	if err := ocr.New(srv.URL).Ping(context.Background()); !errors.Is(err, ocr.ErrUnavailable) {
		t.Errorf("ping of a stopped service = %v", err)
	}
}

func TestWithoutURLEverythingIsUnavailable(t *testing.T) {
	c := ocr.New(" ")
	ctx := context.Background()
	if _, err := c.Read(ctx, ocr.File{}); !errors.Is(err, ocr.ErrUnavailable) {
		t.Errorf("read = %v", err)
	}
	if _, err := c.Render(ctx, ocr.File{}, 1); !errors.Is(err, ocr.ErrUnavailable) {
		t.Errorf("render = %v", err)
	}
	if err := c.Ping(ctx); !errors.Is(err, ocr.ErrUnavailable) {
		t.Errorf("ping = %v", err)
	}
}
