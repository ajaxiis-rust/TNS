package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	minLat = 25.5
	maxLat = 37.5
	minLon = 29.0
	maxLon = 40.5
	zoom   = 8 // 7–9 обычно достаточно

	tileSize = 256
	outDir   = "tiles"
	outImage = "opentopomap_levant.png"
	workers  = 8
)

func deg2num(lat, lon float64, z int) (x, y int) {
	latRad := lat * math.Pi / 180
	n := math.Pow(2, float64(z))
	x = int((lon + 180.0) / 360.0 * n)
	y = int((1.0 - math.Asinh(math.Tan(latRad))/math.Pi) / 2.0 * n)
	return
}

func downloadTile(x, y, z int) error {
	filename := filepath.Join(outDir, fmt.Sprintf("%d_%d_%d.png", z, x, y))
	if _, err := os.Stat(filename); err == nil {
		return nil // уже есть
	}

	url := fmt.Sprintf("https://a.tile.opentopomap.org/%d/%d/%d.png", z, x, y)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func main() {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		panic(err)
	}

	xMin, yMax := deg2num(minLat, minLon, zoom)
	xMax, yMin := deg2num(maxLat, maxLon, zoom)

	type tile struct{ x, y int }
	var tiles []tile
	for x := xMin; x <= xMax; x++ {
		for y := yMin; y <= yMax; y++ {
			tiles = append(tiles, tile{x, y})
		}
	}

	fmt.Printf("Нужно скачать %d тайлов (z=%d)...\n", len(tiles), zoom)

	// Параллельная загрузка
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed int

	for _, t := range tiles {
		wg.Add(1)
		go func(x, y int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := downloadTile(x, y, zoom); err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
				fmt.Printf("ошибка %d/%d: %v\n", x, y, err)
			}
		}(t.x, t.y)
	}
	wg.Wait()

	if failed > 0 {
		fmt.Printf("Не удалось скачать %d тайлов\n", failed)
	}

	// Склеиваем
	width := (xMax - xMin + 1) * tileSize
	height := (yMax - yMin + 1) * tileSize
	result := image.NewRGBA(image.Rect(0, 0, width, height))

	fmt.Println("Склеиваю...")

	for x := xMin; x <= xMax; x++ {
		for y := yMin; y <= yMax; y++ {
			filename := filepath.Join(outDir, fmt.Sprintf("%d_%d_%d.png", zoom, x, y))
			f, err := os.Open(filename)
			if err != nil {
				continue
			}
			img, err := png.Decode(f)
			f.Close()
			if err != nil {
				continue
			}

			pt := image.Pt((x-xMin)*tileSize, (y-yMin)*tileSize)
			draw.Draw(result, img.Bounds().Add(pt), img, image.Point{}, draw.Src)
		}
	}

	out, err := os.Create(outImage)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if err := png.Encode(out, result); err != nil {
		panic(err)
	}

	fmt.Printf("Готово → %s (%dx%d)\n", outImage, width, height)
}
