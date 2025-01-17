package docconv

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/richardlehane/mscfb"
	"github.com/richardlehane/msoleps"
)

// ConvertDoc converts an MS Word .doc to text.
func ConvertDoc(r io.Reader) (string, map[string]string, error) {
	f, err := NewLocalFile(r)
	if err != nil {
		return "", nil, fmt.Errorf("error creating local file: %v", err)
	}
	defer f.Done()

	// Meta data
	mc := make(chan map[string]string, 1)
	go func() {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("panic when reading doc format: %v", e)
				close(mc)
			}
		}()

		meta := make(map[string]string)

		doc, err := mscfb.New(f)
		if err != nil {
			log.Printf("ConvertDoc: could not read doc: %v", err)
			close(mc)
			return
		}

		props := msoleps.New()
		for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
			if msoleps.IsMSOLEPS(entry.Initial) {
				if oerr := props.Reset(doc); oerr != nil {
					log.Printf("ConvertDoc: could not reset props: %v", oerr)
					break
				}

				for _, prop := range props.Property {
					meta[prop.Name] = prop.String()
				}
			}
		}

		const defaultTimeFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

		// Convert parsed meta
		if tmp, ok := meta["LastSaveTime"]; ok {
			if t, err := time.Parse(defaultTimeFormat, tmp); err == nil {
				meta["ModifiedDate"] = fmt.Sprintf("%d", t.Unix())
			}
		}
		if tmp, ok := meta["CreateTime"]; ok {
			if t, err := time.Parse(defaultTimeFormat, tmp); err == nil {
				meta["CreatedDate"] = fmt.Sprintf("%d", t.Unix())
			}
		}

		mc <- meta
	}()

	// Document body
	bc := make(chan string, 1)
	go func() {

		// Save output to a file
		outputFile, err := ioutil.TempFile("/tmp", "docconv-")
		if err != nil {
			log.Println("TempFile Out:", err)
			close(bc)
			return
		}
		defer os.Remove(outputFile.Name())

		err = exec.Command("wvText", f.Name(), outputFile.Name()).Run()
		if err != nil {
			log.Println("wvText exec:", err)
			close(bc)
			return
		}

		var buf bytes.Buffer
		_, err = buf.ReadFrom(outputFile)
		if err != nil {
			log.Println("wvText read:", err)
			close(bc)
			return
		}

		bc <- buf.String()
	}()

	body, bd := <-bc
	meta, md := <-mc

	if bd || md {
		f.Seek(0, 0)
		return ConvertDocx(f)
	}
	return body, meta, nil
}
