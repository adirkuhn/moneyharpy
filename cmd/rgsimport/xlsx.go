package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// readSheet returns the rows of the named worksheet as text cells. It reads
// just enough of the Office Open XML format for RGS: shared and inline
// strings and plain values.
func readSheet(r io.ReaderAt, size int64, name string) ([][]string, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("open workbook: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	var shared []string
	if f, ok := files["xl/sharedStrings.xml"]; ok {
		var sst struct {
			Items []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := decode(f, &sst); err != nil {
			return nil, err
		}
		for _, si := range sst.Items {
			text := si.T
			for _, run := range si.R {
				text += run.T
			}
			shared = append(shared, text)
		}
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := decode(files["xl/workbook.xml"], &wb); err != nil {
		return nil, err
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := decode(files["xl/_rels/workbook.xml.rels"], &rels); err != nil {
		return nil, err
	}
	target := ""
	for _, s := range wb.Sheets {
		if s.Name != name {
			continue
		}
		for _, rel := range rels.Rels {
			if rel.ID == s.ID {
				target = path.Join("xl", strings.TrimPrefix(rel.Target, "/xl/"))
			}
		}
	}
	f, ok := files[target]
	if target == "" || !ok {
		return nil, fmt.Errorf("workbook has no sheet %q", name)
	}
	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := decode(f, &ws); err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(ws.Rows))
	for _, row := range ws.Rows {
		var cells []string
		for _, c := range row.Cells {
			col := column(c.Ref)
			for len(cells) <= col {
				cells = append(cells, "")
			}
			switch c.Type {
			case "s":
				i, err := strconv.Atoi(c.Value)
				if err != nil || i >= len(shared) {
					return nil, fmt.Errorf("cell %s: bad shared string %q", c.Ref, c.Value)
				}
				cells[col] = shared[i]
			case "inlineStr":
				cells[col] = c.Inline
			default:
				cells[col] = c.Value
			}
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

func decode(f *zip.File, v any) error {
	if f == nil {
		return fmt.Errorf("workbook part missing")
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return xml.NewDecoder(rc).Decode(v)
}

// column turns a cell reference such as "C12" into a zero-based column.
func column(ref string) int {
	n := 0
	for _, ch := range ref {
		if ch < 'A' || ch > 'Z' {
			break
		}
		n = n*26 + int(ch-'A'+1)
	}
	return n - 1
}
