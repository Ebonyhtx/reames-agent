package themepack

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"strings"

	"golang.org/x/image/webp"
)

type imageInfo struct {
	MIME   string
	Ext    string
	Width  int
	Height int
}

func validateImage(data []byte, name string) (imageInfo, error) {
	if len(data) == 0 {
		return imageInfo{}, fmt.Errorf("image %q is empty", name)
	}
	if len(data) > MaxImageBytes {
		return imageInfo{}, fmt.Errorf("image %q exceeds %d bytes", name, MaxImageBytes)
	}
	mime, ext := sniffImage(data, name)
	if mime == "" {
		return imageInfo{}, fmt.Errorf("image %q must have matching PNG, JPEG, or WebP content and extension", name)
	}
	reader := bytes.NewReader(data)
	var (
		cfg    image.Config
		format string
		err    error
	)
	if mime == "image/webp" {
		cfg, err = webp.DecodeConfig(reader)
		format = "webp"
	} else {
		cfg, format, err = image.DecodeConfig(reader)
	}
	if err != nil {
		return imageInfo{}, fmt.Errorf("decode image %q: %w", name, err)
	}
	if (mime == "image/png" && format != "png") || (mime == "image/jpeg" && format != "jpeg") || (mime == "image/webp" && format != "webp") {
		return imageInfo{}, fmt.Errorf("image %q decoded as unexpected format %q", name, format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageEdge || cfg.Height > MaxImageEdge {
		return imageInfo{}, fmt.Errorf("image %q dimensions %dx%d exceed %dx%d", name, cfg.Width, cfg.Height, MaxImageEdge, MaxImageEdge)
	}
	if uint64(cfg.Width)*uint64(cfg.Height) > MaxImagePixels {
		return imageInfo{}, fmt.Errorf("image %q exceeds %d pixels", name, MaxImagePixels)
	}
	return imageInfo{MIME: mime, Ext: ext, Width: cfg.Width, Height: cfg.Height}, nil
}

func sniffImage(data []byte, name string) (string, string) {
	ext := strings.ToLower(filepath.Ext(name))
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) && ext == ".png" {
		return "image/png", ".png"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff && (ext == ".jpg" || ext == ".jpeg") {
		return "image/jpeg", ".jpg"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" && ext == ".webp" {
		return "image/webp", ".webp"
	}
	return "", ""
}

func readLimited(reader io.Reader, max int64, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, max)
	}
	return data, nil
}
