package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
)

type tableXML struct {
	Name string   `xml:"name,attr"`
	Tags []tagXML `xml:"tag"`
}

type tagXML struct {
	Name         string    `xml:"name,attr"`
	Type         string    `xml:"type,attr"`
	Writable     bool      `xml:"writable,attr"`
	Descriptions []descXML `xml:"desc"`
}

type descXML struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

type tagJSON struct {
	Type        string            `json:"type"`
	Writable    bool              `json:"writable"`
	Path        string            `json:"path"`
	Group       string            `json:"group"`
	Description map[string]string `json:"description"`
}

func tagsStreamer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Setup http streaming
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Context-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	w.Write([]byte("[\n"))

	pr, pw := io.Pipe()
	defer pw.Close()

	cmd := exec.CommandContext(ctx, "exiftool", "-listx")
	cmd.Stdout = pw
	cmd.Start()

	go func() {
		defer func() {
			recover()
		}()

		xmlDecoder := xml.NewDecoder(pr)
		jsonDecoder := json.NewEncoder(w)
		tagsCount := 0
		for {
			t, err := xmlDecoder.Token()
			if t == nil {
				break
			}

			if err == io.EOF {
				break
			}

			// We're going to stream one table at a time
			switch s := t.(type) {
			case xml.StartElement:
				if s.Name.Local == "table" {
					table := &tableXML{}
					xmlDecoder.DecodeElement(table, &s)
					for _, t := range table.Tags {
						if tagsCount != 0 {
							w.Write([]byte(","))
						}
						tagsCount++
						var tag tagJSON
						tag.Path = fmt.Sprintf("%s:%s", table.Name, t.Name)
						tag.Group = table.Name
						tag.Type = t.Type
						tag.Writable = t.Writable
						tag.Description = make(map[string]string)
						for _, d := range t.Descriptions {
							tag.Description[d.Lang] = d.Value
						}
						err := jsonDecoder.Encode(tag)
						if err != nil {
							break
						}
					}
				}
			}
		}

		w.Write([]byte("]\n"))
		log.Printf("Streamed %d tags\n", tagsCount)
	}()

	cmd.Wait()
}

func main() {
	_, err := exec.LookPath("exiftool")
	if err != nil {
		log.Fatalln("Please make sure exiftool is installed and is in your PATH")
	}

	http.HandleFunc("/tags", tagsStreamer)
	log.Fatalln(http.ListenAndServe(":1002", nil))
}
