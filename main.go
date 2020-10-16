package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
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
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming disabled - Please try again at a later time",
			http.StatusPreconditionFailed)
		return
	}

	// Setup http streaming
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	fmt.Fprint(w, "[")
	flusher.Flush()

	pr, pw := io.Pipe()
	defer pw.Close()

	// TODO(@hady): If this is a production level service in a tiered environment, where this ranks high
	// I wouldn't simply ignore the possibility of an error taking place here.
	go func() {
		defer func() { recover() }()
		ctx := r.Context()
		cmd := exec.CommandContext(ctx, exiftoolPath, "-listx")
		cmd.Stdout = pw
		cmd.Run()
	}()

	xmlDecoder := xml.NewDecoder(pr)
	tagsCount := 0

DECODERLOOP:
	for {
		currToken, err := xmlDecoder.Token()
		// Could break only on EOF but honestly, any issue with tokenization should kill the whole request here
		if err != nil {
			break
		}

		switch elm := currToken.(type) {
		case xml.StartElement:
			if elm.Name.Local == "table" {
				table := &tableXML{}
				xmlDecoder.DecodeElement(table, &elm)

				for _, tag := range table.Tags {
					if tagsCount != 0 {
						fmt.Fprint(w, ",")
					}

					t := tagJSON{
						Path:        fmt.Sprintf("%s:%s", table.Name, tag.Name),
						Group:       table.Name,
						Type:        tag.Type,
						Writable:    tag.Writable,
						Description: make(map[string]string),
					}
					for _, desc := range tag.Descriptions {
						t.Description[desc.Lang] = desc.Value
					}

					tJSON, err := json.Marshal(t)
					if err != nil {
						continue
					}
					fmt.Fprint(w, string(tJSON))
					tagsCount++
				}
				flusher.Flush()
			}
		case xml.EndElement:
			if elm.Name.Local == "taginfo" {
				break DECODERLOOP
			}
		}
	}

	fmt.Fprint(w, "]")
	flusher.Flush()

}

type color string

const (
	cblack  color = "\u001b[30m"
	cred          = "\u001b[31m"
	cgreen        = "\u001b[32m"
	cblue         = "\u001b[34m"
	cyellow       = "\u001b[33m"
	creset        = "\u001b[0m"
)

func colPrintlnf(c color, sfmt string, v ...interface{}) {
	fmt.Printf("%s%s%s\n", string(c), fmt.Sprintf(sfmt, v...), string(creset))
}

var (
	listenAddr   string
	exiftoolPath string
)

func main() {
	flag.StringVar(&listenAddr, "bind-address", "127.0.0.1:3000", "Address to bind the server to")
	flag.StringVar(&exiftoolPath, "exiftool", "", "Path to exiftool")
	flag.Parse()

	if len(exiftoolPath) == 0 {
		var err error
		exiftoolPath, err = exec.LookPath("exiftool")
		if err != nil {
			colPrintlnf(cred, "No exiftool found in your PATH, please provide one")
		}
	}

	r := http.NewServeMux()
	r.HandleFunc("/tags", tagsStreamer)
	r.Handle("/", http.RedirectHandler("tags", http.StatusSeeOther))

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		WriteTimeout: 25 * time.Second,
		IdleTimeout:  5 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)

	go func() {
		<-quit
		colPrintlnf(cyellow, "Shutting web server down")
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			colPrintlnf(cred, "Couldn't shut web server down.\nError=%s", err.Error())
		}
	}()

	colPrintlnf(cgreen, "Accepting requests on address: %s", listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		colPrintlnf(cred, "Couldn't bind server to %s.\nError=%s", listenAddr, err.Error())
	}

	colPrintlnf(cgreen, "Bye.")
}
