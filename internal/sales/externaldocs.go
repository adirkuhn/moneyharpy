package sales

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ExternalPDF renders a booked external invoice from its data in the
// administration's default document style, sender and language, for an
// invoice whose own file is not attached. Its reference is its number.
func (s *Service) ExternalPDF(ctx context.Context, admin, id, user resource.ID) ([]byte, string, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, "", err
	}
	e, err := s.ExternalInvoice(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	if !e.Booked() {
		return nil, "", invalid("base", "Fill in the invoice first; a new invoice has no contact or lines yet")
	}
	inv := e.invoice()
	inv.InvoiceID, inv.Language = externalNumber(e), a.Language
	styles, err := s.invoicing.DocumentStyles(ctx, admin, user)
	if err != nil {
		return nil, "", wrap("load document styles", err)
	}
	if i := slices.IndexFunc(styles, func(d invoicing.DocumentStyle) bool { return d.Default }); i >= 0 {
		inv.DocumentStyleID = styles[i].ID
	}
	doc, err := s.document(ctx, a, user, inv)
	if err != nil {
		return nil, "", err
	}
	data, err := s.renderer.PDF(doc)
	return data, fileName(inv, "pdf"), wrap("render external invoice", err)
}

// ExternalAttachmentsZip packs the files of an external invoice in one ZIP
// archive.
func (s *Service) ExternalAttachmentsZip(ctx context.Context, admin, id, user resource.ID) ([]byte, string, error) {
	e, err := s.ExternalInvoice(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	if len(e.Attachments) == 0 {
		return nil, "", invalid("base", "The invoice has no attachments")
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	used := map[string]bool{}
	for _, att := range e.Attachments {
		meta, body, err := s.attachments.Open(ctx, admin, ExternalEntityType, id, att.ID)
		if err != nil {
			return nil, "", wrap("open attachment", err)
		}
		w, err := zw.Create(uniqueName(meta.Filename, used))
		if err == nil {
			_, err = io.Copy(w, body)
		}
		_ = body.Close()
		if err != nil {
			return nil, "", wrap("pack attachment", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", wrap("pack attachments", err)
	}
	return out.Bytes(), fileName(Invoice{InvoiceID: externalNumber(e)}, "zip"), nil
}

// externalNumber is how the invoice is known: its reference, or its entry
// number when it has none.
func externalNumber(e ExternalInvoice) string {
	if e.Reference != "" {
		return e.Reference
	}
	return strconv.Itoa(e.EntryNumber)
}

// uniqueName is the file's base name, numbered when an earlier file of the
// archive has it already.
func uniqueName(name string, used map[string]bool) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" || name == "" {
		name = "attachment"
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for n := 2; used[candidate]; n++ {
		candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
	}
	used[candidate] = true
	return candidate
}
