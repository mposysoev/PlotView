package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-sixel"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// -----------------------------------------------------------------------------
// Default Constants
// -----------------------------------------------------------------------------

const (
	defaultWidth     = 1200 // Default plot width in points
	defaultHeight    = 1200 // Default plot height in points
	defaultScale     = 1.0  // Default scale factor for SIXEL output
	defaultLineWidth = 1.0  // Default line width in points
)

// -----------------------------------------------------------------------------
// Default Colors
// -----------------------------------------------------------------------------

var (
	defaultColors = struct {
		line       color.Color
		scatter    color.Color
		background color.Color
	}{
		// Red line and scatter points
		line:    color.RGBA{R: 0, G: 0, B: 0, A: 255},
		scatter: color.RGBA{R: 0, G: 0, B: 0, A: 255},
		// White background
		background: color.RGBA{R: 255, G: 255, B: 255, A: 255},
	}
)

// -----------------------------------------------------------------------------
// Config and Data Types
// -----------------------------------------------------------------------------

// Config holds all user-configurable parameters for plotting.
type Config struct {
	Width, Height int     // Dimensions of the plot in points
	Scale         float64 // Scale factor for SIXEL output
	Input         string  // Input data file

	LineWidth float64 // Width of the plot line in points

	// Colors for different plot elements
	Colors struct {
		Line, Scatter, Background color.Color
	}
}

// Point represents a single (X, Y) coordinate.
type Point struct {
	X, Y float64
}

// -----------------------------------------------------------------------------
// Main Entry Point
// -----------------------------------------------------------------------------

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

// parseFlags defines and parses command-line flags,
// assigning their values into a Config struct.
func parseFlags() Config {
	var cfg Config

	// Define CLI flags with usage text
	flag.IntVar(&cfg.Width, "w", defaultWidth, "plot width in points")
	flag.IntVar(&cfg.Height, "h", defaultHeight, "plot height in points")
	flag.Float64Var(&cfg.Scale, "s", defaultScale, "SIXEL scale factor")
	flag.Float64Var(&cfg.LineWidth, "line-width", defaultLineWidth, "line width in points")

	flag.Parse()

	// Expect exactly one input filename
	if flag.NArg() != 1 {
		log.Fatal("Usage: plotter [options] data_file")
	}

	// Set Config fields
	cfg.Input = flag.Arg(0)
	cfg.Colors.Line = defaultColors.line
	cfg.Colors.Scatter = defaultColors.scatter
	cfg.Colors.Background = defaultColors.background

	return cfg
}

// run orchestrates reading the data file, creating a plot, and optionally
// displaying the resulting image via SIXEL if the terminal supports it.
func run(cfg Config) error {
	points, err := readData(cfg.Input)
	if err != nil {
		return fmt.Errorf("reading data from %q: %w", cfg.Input, err)
	}
	if len(points) == 0 {
		return fmt.Errorf("no valid data points found in %q", cfg.Input)
	}

	// Construct output filename, e.g. "data_plot.png"
	outFile := strings.TrimSuffix(cfg.Input, filepath.Ext(cfg.Input)) + "_plot.png"

	if err := createPlot(points, outFile, cfg); err != nil {
		return fmt.Errorf("creating plot: %w", err)
	}
	log.Printf("Plot saved to: %s", outFile)

	// Attempt to display the plot via SIXEL
	if err := displaySixel(outFile, cfg); err != nil {
		return fmt.Errorf("displaying SIXEL: %w", err)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Reading Data
// -----------------------------------------------------------------------------

// readData opens the given file, reads it line-by-line, and converts each line
// into either (X, Y) or (lineIndex, Y). Lines starting with '#' or '%'
// (or blank lines) are treated as comments and skipped.
func readData(filename string) ([]Point, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var (
		points    []Point
		scanner   = bufio.NewScanner(file)
		lineIndex float64
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Ignore empty lines or lines starting with '#' or '%'
		if line == "" || line[0] == '#' || line[0] == '%' {
			continue
		}

		point, err := parseLine(line, lineIndex)
		if err != nil {
			// Log and continue rather than abort on malformed lines
			log.Printf("Skipping line %.0f in %s: %v", lineIndex+1, filename, err)
			continue
		}
		points = append(points, point)
		lineIndex++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}
	return points, nil
}

// parseLine attempts to parse one line of text into either:
//
//	(1) a single float (treated as Y, with X = lineIndex), or
//	(2) two floats (treated as X and Y).
func parseLine(line string, lineIndex float64) (Point, error) {
	fields := strings.Fields(line)

	switch len(fields) {
	case 1:
		// One field => interpret as Y, with X = lineIndex
		y, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return Point{}, fmt.Errorf("invalid Y value %q", fields[0])
		}
		return Point{X: lineIndex, Y: y}, nil

	case 2:
		// Two fields => interpret as (X, Y)
		x, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return Point{}, fmt.Errorf("invalid X value %q", fields[0])
		}
		y, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return Point{}, fmt.Errorf("invalid Y value %q", fields[1])
		}
		return Point{X: x, Y: y}, nil

	default:
		// More than 2 fields or 0 => not supported
		return Point{}, fmt.Errorf("expected 1 or 2 values, got %d", len(fields))
	}
}

// -----------------------------------------------------------------------------
// Creating and Saving the Plot
// -----------------------------------------------------------------------------

// createPlot builds a PNG plot from the data points and saves it to outFile.
func createPlot(points []Point, outFile string, cfg Config) error {
	p := plot.New()
	p.Title.Text = "Data Plot"
	p.X.Label.Text = "X"
	p.Y.Label.Text = "Y"

	// Set background color
	p.BackgroundColor = cfg.Colors.Background

	// Convert our []Point slice into a plotter.XYs
	pts := make(plotter.XYs, len(points))
	for i, pt := range points {
		pts[i].X = pt.X
		pts[i].Y = pt.Y
	}

	line, scatter, err := createPlotters(pts, cfg)
	if err != nil {
		return fmt.Errorf("creating plotters: %w", err)
	}

	// Add the line and scatter plotter to the plot
	p.Add(line, scatter)

	// Save the plot as PNG with the given width/height
	if err := p.Save(vg.Points(float64(cfg.Width)), vg.Points(float64(cfg.Height)), outFile); err != nil {
		return fmt.Errorf("save plot: %w", err)
	}
	return nil
}

// createPlotters initializes a line and scatter plotter with appropriate colors
// and line width.
func createPlotters(pts plotter.XYs, cfg Config) (*plotter.Line, *plotter.Scatter, error) {
	// Create a line plotter
	line, err := plotter.NewLine(pts)
	if err != nil {
		return nil, nil, fmt.Errorf("create line plotter: %w", err)
	}
	line.Color = cfg.Colors.Line
	line.Width = vg.Points(cfg.LineWidth) // Apply line width

	// Create a scatter plotter
	scatter, err := plotter.NewScatter(pts)
	if err != nil {
		return nil, nil, fmt.Errorf("create scatter plotter: %w", err)
	}
	scatter.GlyphStyle.Color = cfg.Colors.Scatter
	scatter.GlyphStyle.Radius = 2

	return line, scatter, nil
}

// -----------------------------------------------------------------------------
// SIXEL Display
// -----------------------------------------------------------------------------

// displaySixel attempts to display the resulting plot via SIXEL,
// adjusting image size if the user has specified a scale factor.
func displaySixel(filename string, cfg Config) error {
	if !isSixelSupported() {
		return nil
	}

	imgFile, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open image file: %w", err)
	}
	defer imgFile.Close()

	img, _, err := image.Decode(imgFile)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	enc := sixel.NewEncoder(os.Stdout)
	if cfg.Scale != 1.0 {
		enc.Width = int(float64(cfg.Width) * cfg.Scale)
		enc.Height = int(float64(cfg.Height) * cfg.Scale)
	}

	if err := enc.Encode(img); err != nil {
		return fmt.Errorf("encode SIXEL: %w", err)
	}
	return nil
}

// isSixelSupported checks for a terminal type known to support SIXEL.
func isSixelSupported() bool {
	term := strings.ToLower(os.Getenv("TERM"))
	return strings.Contains(term, "xterm") ||
		strings.Contains(term, "vt340") ||
		strings.Contains(term, "mlterm")
}
