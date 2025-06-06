package csvutils

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"unicode"

	"github.com/anvesh9652/pgload/internal/pgdb/dbv2"
	"github.com/anvesh9652/pgload/pkg/shared"
	"github.com/anvesh9652/pgload/pkg/shared/reader"
)

func NewCSVReaderAndColumns(path string) (*csv.Reader, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	csvr := csv.NewReader(file)
	headers, err := csvr.Read()
	return csvr, headers, err
}

func BuildColumnTypeStr(types map[string]string) (res []string) {
	for col, tp := range types {
		res = append(res, col+" "+tp)
	}
	return
}

func FindColumnTypes(path string, lookUpSize int, typeSetting *string) (map[string]string, error) {
	r, err := reader.NewFileGzipReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	headers, br, err := GetCSVHeaders(r)
	csvr := csv.NewReader(br)
	if err != nil {
		return nil, err
	}

	var lookUpRows [][]string
	for range lookUpSize {
		record, err := csvr.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		lookUpRows = append(lookUpRows, record)
	}

	rowsCount := len(lookUpRows)
	types := map[string]string{}
	for i, col := range headers {
		typesCnt := map[string]int{}
		for ix := range rowsCount {
			val := lookUpRows[ix][i]
			if val != "" {
				typesCnt[findType(val, typeSetting)] += 1
			}
		}
		types[col] = maxRecordedType(typesCnt)
	}
	return types, nil
}

func maxRecordedType(types map[string]int) string {
	if types[dbv2.Text] > 0 {
		return dbv2.Text
	}
	val, res := -1, dbv2.Text
	for k, v := range types {
		if v > val {
			val, res = v, k
		}
	}
	return res
}

func findType(val string, typeSetting *string) string {
	if typeSetting != nil && *typeSetting == shared.AllText {
		return dbv2.Text
	}

	if _, err := strconv.ParseInt(val, 10, 64); err == nil {
		return dbv2.Numeric
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return dbv2.Numeric
	}
	return dbv2.Text
}

func GetCSVHeaders(r io.Reader) ([]string, io.Reader, error) {
	// Didn't find the best way to get only the first row.
	// No need to worry here if `br` reads more than the first row.
	br := bufio.NewReader(r)
	buff := bytes.NewBuffer(nil)
	for {
		line, prefix, err := br.ReadLine()
		if err != nil {
			return nil, nil, err
		}
		buff.Write(line)
		if !prefix {
			break
		}
	}
	csvR := csv.NewReader(buff)
	headers, err := csvR.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read first line: %v", err)
	}

	return preserveExactColNames(headers), br, nil
}

// Preserve the exact column names by quoting them.
func preserveExactColNames(headers []string) []string {
	var quotedCols []string
	for _, orgCol := range headers {
		quotedCols = append(quotedCols, strconv.Quote(orgCol))
	}
	return quotedCols
}

func isLetterDigit(r rune) bool {
	return unicode.IsDigit(r) || unicode.IsLetter(r)
}
