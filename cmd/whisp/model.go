package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tamasmagyar/whisp/internal/config"
)

const modelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin"

func downloadModel(name string) error {
	dst := config.ModelPath(name)
	if _, err := os.Stat(dst); err == nil {
		fmt.Println("model already present:", dst)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	url := fmt.Sprintf(modelURL, name)
	fmt.Println("downloading", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	pw := &progress{total: resp.ContentLength, last: time.Now()}
	_, err = io.Copy(f, io.TeeReader(resp.Body, pw))
	f.Close()
	fmt.Println()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	fmt.Println("saved", dst)
	return nil
}

type progress struct {
	total, done int64
	last        time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if time.Since(p.last) > 200*time.Millisecond || p.done == p.total {
		p.last = time.Now()
		if p.total > 0 {
			fmt.Printf("\r  %.0f / %.0f MB (%.0f%%)", float64(p.done)/1e6, float64(p.total)/1e6, 100*float64(p.done)/float64(p.total))
		} else {
			fmt.Printf("\r  %.0f MB", float64(p.done)/1e6)
		}
	}
	return len(b), nil
}
