package jsonloader

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"sync"
	// jsoniter "github.com/json-iterator/go"
)

// var jsoni = jsoniter.ConfigFastest

const (
	chanSize = 50
	buffSize = 10 * 1024 * 1024 // 10 MB

	numWorkers = 5
)

type AsyncReader struct {
	cols []string
	r    io.Reader

	ErrCh chan error

	OutCh chan []string
	InCh  chan []byte

	pool sync.Pool
}

func NewAsyncReader(r io.Reader, cw *csv.Writer, cols []string) *AsyncReader {
	w := &AsyncReader{
		cols:  cols,
		r:     r,
		ErrCh: make(chan error, 10),
		InCh:  make(chan []byte, chanSize),
		OutCh: make(chan []string, chanSize),
		pool: sync.Pool{
			New: func() any {
				buff := bytes.NewBuffer(nil)
				return buff
			},
		},
	}

	buff := make([]byte, buffSize)
	var leftOver []byte

	go func() {
		for {
			n, err := r.Read(buff)
			if err != nil {
				if err == io.EOF {
					break
				}
				w.ErrCh <- err
				break
			}

			lastNewLineIdx := bytes.LastIndex(buff[:n], []byte("\n"))
			if lastNewLineIdx == -1 {
				leftOver = append(leftOver, buff[:n]...)
				continue
			}
			merged := append(leftOver, buff[:lastNewLineIdx]...)
			leftOver = make([]byte, n-lastNewLineIdx-1)
			copy(leftOver, buff[lastNewLineIdx+1:n])
			w.InCh <- merged
		}
		close(w.InCh)
	}()
	return w
}

// just got to know that writer are not thread safe in go
func (a *AsyncReader) parseRows() {
	wg := new(sync.WaitGroup)
	defer close(a.OutCh)
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for data := range a.InCh {
				buff := a.pool.Get().(*bytes.Buffer)
				buff.Reset()
				if _, err := buff.Write(data); err != nil {
					a.ErrCh <- err
					return
				}
				dec := json.NewDecoder(buff)
				if err := a.sendToOutput(dec); err != nil {
					a.ErrCh <- err
					return
				}
				a.pool.Put(buff)
			}
		}()
	}
	wg.Wait()
}

func (a *AsyncReader) sendToOutput(dec *json.Decoder) error {
	var err error
	for dec.More() {
		var r row
		if err = dec.Decode(&r); err != nil {
			return err
		}
		var csvRow = make([]string, len(a.cols))
		for i, header := range a.cols {
			csvRow[i] = toString(r[header])
		}
		a.OutCh <- csvRow
	}
	return nil
}