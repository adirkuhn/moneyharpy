// Command rgsimport turns the Dutch Referentie Grootboekschema (RGS), as
// published in Excel at https://www.referentiegrootboekschema.nl, into the
// CSV that MoneyHarpy embeds.
//
// RGS is published for use free of licence costs; its maintainers adopted
// Creative Commons Zero for it. MoneyHarpy uses the MKB (small business)
// edition.
//
// Usage:
//
//	go run ./cmd/rgsimport -in "RGS 3.8-def.xlsx" -version 3.8 -out internal/taxonomy/data/nl-rgs-3.8.csv
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	in := flag.String("in", "", "RGS Excel file")
	version := flag.String("version", "", "RGS version, such as 3.8")
	out := flag.String("out", "", "CSV to write")
	sheet := flag.String("sheet", "", "worksheet; defaults to MKB-RGS<version>-def")
	flag.Parse()
	if *in == "" || *version == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *sheet == "" {
		*sheet = "MKB-RGS" + *version + "-def"
	}
	if err := run(*in, *sheet, *out); err != nil {
		fmt.Fprintln(os.Stderr, "rgsimport:", err)
		os.Exit(1)
	}
}

func run(in, sheet, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	rows, err := readSheet(f, info.Size(), sheet)
	if err != nil {
		return err
	}
	items, err := convert(rows)
	if err != nil {
		return err
	}
	w, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := write(w, items); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// item is one RGS code as MoneyHarpy stores it.
type item struct {
	code, reference, name, description, debitCredit, parent string
	level                                                   int
	basic                                                   bool
}

var header = []string{"code", "reference", "name", "description", "debit_credit", "level", "parent", "basic"}

// convert reads the RGS columns by their header names and links every code
// to the nearest code above it at a lower level, which is how RGS nests.
func convert(rows [][]string) ([]item, error) {
	head := -1
	for i, r := range rows {
		if len(r) > 0 && r[0] == "Referentiecode" {
			head = i
			break
		}
	}
	if head < 0 {
		return nil, errors.New("no Referentiecode header row")
	}
	col := map[string]int{}
	for i, name := range rows[head] {
		if _, seen := col[name]; !seen {
			col[name] = i
		}
	}
	for _, name := range []string{"Referentiecode", "Referentienummer", "Omschrijving (verkort)", "Omschrijving", "D/C", "Nivo", "Basis"} {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("no %q column", name)
		}
	}
	cell := func(r []string, name string) string {
		if i := col[name]; i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}
	var items []item
	stack := map[int]string{} // level -> latest code at that level
	for _, r := range rows[head+1:] {
		code := cell(r, "Referentiecode")
		if code == "" {
			continue
		}
		level, err := strconv.Atoi(cell(r, "Nivo"))
		if err != nil {
			return nil, fmt.Errorf("%s: level %q", code, cell(r, "Nivo"))
		}
		parent := ""
		for l := level - 1; l > 0; l-- {
			if p, ok := stack[l]; ok {
				parent = p
				break
			}
		}
		stack[level] = code
		for l := level + 1; l <= 9; l++ {
			delete(stack, l)
		}
		items = append(items, item{
			code: code, reference: cell(r, "Referentienummer"), name: cell(r, "Omschrijving (verkort)"),
			description: cell(r, "Omschrijving"), debitCredit: cell(r, "D/C"), parent: parent, level: level,
			basic: cell(r, "Basis") == "1",
		})
	}
	if len(items) == 0 {
		return nil, errors.New("no codes found")
	}
	return items, nil
}

func write(w *os.File, items []item) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, it := range items {
		if err := cw.Write([]string{
			it.code, it.reference, it.name, it.description, it.debitCredit, strconv.Itoa(it.level), it.parent, strconv.FormatBool(it.basic),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
