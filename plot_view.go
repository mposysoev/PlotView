package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-sixel"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

type Config struct {
	width  int
	height int
	scale  float64
}

type DataPoint struct {
	X, Y float64
}

func main() {
	// Command-line parameter setup
	config := Config{}
	flag.IntVar(&config.width, "width", 800, "Output width")
	flag.IntVar(&config.height, "height", 600, "Output height")
	flag.Float64Var(&config.scale, "scale", 1.0, "Scale factor (default: 1.0)")

	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] data_file\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	dataPath := flag.Arg(0)
	if err := plotDataToSixel(dataPath, config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func readDataFromFile(filename string) ([]DataPoint, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	var points []DataPoint
	scanner := bufio.NewScanner(file)
	lineNumber := 0.0
	lineCount := 0

	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		// Skip comments
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "/") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		var point DataPoint
		if len(fields) >= 2 {
			x, errX := strconv.ParseFloat(fields[0], 64)
			y, errY := strconv.ParseFloat(fields[1], 64)
			if errX != nil || errY != nil {
				fmt.Printf("Parsing error on line %d: %s\n", lineCount, line)
				continue
			}
			point = DataPoint{X: x, Y: y}
		} else if len(fields) == 1 {
			y, err := strconv.ParseFloat(fields[0], 64)
			if err != nil {
				fmt.Printf("Parsing error on line %d: %s\n", lineCount, line)
				continue
			}
			point = DataPoint{X: lineNumber, Y: y}
		} else {
			fmt.Printf("Unsupported format on line %d: %s\n", lineCount, line)
			continue
		}

		points = append(points, point)
		lineNumber += 1.0
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	return points, nil
}

func createPlot(data []DataPoint) (*plot.Plot, error) {
	p := plot.New()

	p.Title.Text = "Data Plot"
	p.X.Label.Text = "X"
	p.Y.Label.Text = "Y"

	// Create points for the plot
	pts := make(plotter.XYs, len(data))
	for i, point := range data {
		pts[i].X = point.X
		pts[i].Y = point.Y
	}

	// Create a line
	line, err := plotter.NewLine(pts)
	if err != nil {
		return nil, fmt.Errorf("error creating line: %v", err)
	}
	line.Color = color.RGBA{R: 255, B: 0, A: 255}

	// Add the line to the plot
	p.Add(line)
	p.Legend.Add("data", line)

	// Add scatter points for better visualization
	scatter, err := plotter.NewScatter(pts)
	if err != nil {
		return nil, fmt.Errorf("error creating scatter: %v", err)
	}
	scatter.GlyphStyle.Color = color.RGBA{R: 255, B: 0, A: 255}
	scatter.GlyphStyle.Radius = 2
	p.Add(scatter)

	return p, nil
}

func plotDataToSixel(filename string, config Config) error {
	// Read data from file
	data, err := readDataFromFile(filename)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return fmt.Errorf("no valid data points found in file")
	}

	// Create the plot
	p, err := createPlot(data)
	if err != nil {
		return err
	}

	// Save the plot to a file
	outFileName := strings.TrimSuffix(filename, filepath.Ext(filename)) + "_plot.png"

	// Convert dimensions from pixels to vg.Length units (points, where 1 point = 1/72 inch)
	width := vg.Points(float64(config.width))
	height := vg.Points(float64(config.height))

	if err := p.Save(width, height, outFileName); err != nil {
		return fmt.Errorf("failed to save plot: %v", err)
	}

	fmt.Println("Plot saved to file:", outFileName)

	// Check for Sixel support
	if isSixelSupported() {
		// Open the saved image
		imgFile, err := os.Open(outFileName)
		if err != nil {
			return fmt.Errorf("failed to open saved image: %v", err)
		}
		defer imgFile.Close()

		img, _, err := image.Decode(imgFile)
		if err != nil {
			return fmt.Errorf("failed to decode saved image: %v", err)
		}

		// Create the encoder
		enc := sixel.NewEncoder(os.Stdout)

		// Apply scaling if needed
		if config.scale != 1.0 {
			newWidth := int(float64(config.width) * config.scale)
			newHeight := int(float64(config.height) * config.scale)
			if newWidth > 0 && newHeight > 0 {
				enc.Width = newWidth
				enc.Height = newHeight
			}
		}

		// Encode and output
		if err := enc.Encode(img); err != nil {
			return fmt.Errorf("failed to encode sixel: %v", err)
		}
	} else {
		fmt.Println("Terminal does not support Sixel. Plot saved to file:", outFileName)
	}

	return nil
}

func isSixelSupported() bool {
	term := os.Getenv("TERM")
	return strings.Contains(term, "xterm") || strings.Contains(term, "vt340") || strings.Contains(term, "mlterm")
}
