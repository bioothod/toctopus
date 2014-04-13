package main

import (
	"bufio"
	"code.google.com/p/draw2d/draw2d"
	"encoding/csv"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"strconv"
)

func ema(value, oldValue, fdtime, ftime float64) float64 {
	alpha := 1.0 - math.Exp(-fdtime/ftime)
	r := alpha*value + (1.0-alpha)*oldValue
	return r
}

func saveToPngFile(filePath string, m image.Image) {
	f, err := os.Create(filePath)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	b := bufio.NewWriter(f)
	err = png.Encode(b, m)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	err = b.Flush()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s OK.\n", filePath)
}

type tset struct {
	price        []float64
	mean_short   []float64
	window_short float64

	mean_long   []float64
	window_long float64

	min, max float64
}

func (v *tset) add_mean(mean []float64, window, close_price float64) []float64 {
	var prev float64
	if len(mean) == 0 {
		prev = close_price
	} else {
		prev = mean[len(mean)-1]
	}
	mean_val := ema(close_price, prev, 1, window)
	return append(mean, mean_val)
}

func (v *tset) add_price(close_price float64) {
	v.price = append(v.price, close_price)

	v.mean_short = v.add_mean(v.mean_short, v.window_short, close_price)
	v.mean_long = v.add_mean(v.mean_long, v.window_long, close_price)

	if close_price > v.max {
		v.max = close_price
	}

	if close_price < v.min {
		v.min = close_price
	}
}

func (v *tset) draw(path string) {
	img := image.NewRGBA(image.Rect(0, 0, len(v.price), 1000))
	gc := draw2d.NewGraphicContext(img)
	gc.Clear()
	gc.SetStrokeColor(image.Black)
	gc.SetFillColor(image.White)
	gc.SetLineWidth(1)
	gc.SetLineCap(draw2d.ButtCap)
	gc.FillStroke()

	ymax := float64(img.Bounds().Max.Y)
	scale := ymax / (v.max - v.min)
	median := (v.max + v.min) / 2.0

	fmt.Printf("scale: %f, min: %f, max: %f\n", scale, v.min, v.max)
	xmax := len(v.price)
	if xmax > img.Bounds().Max.X {
		xmax = img.Bounds().Max.X
	}

	gc.MoveTo(0, ymax)
	for i := 0; i < xmax; i++ {
		x := float64(i)
		y := ymax - ((v.price[i]-v.min)*scale + median)

		gc.LineTo(x, y)
		gc.MoveTo(x, y)
	}
	gc.Stroke()

	gc.MoveTo(0, ymax)
	gc.SetStrokeColor(color.NRGBA{255, 0, 0, 255})
	gc.FillStroke()
	for i := 0; i < xmax; i++ {
		x := float64(i)
		y := ymax - ((v.mean_short[i]-v.min)*scale + median)

		gc.LineTo(x, y)
		gc.MoveTo(x, y)
	}
	gc.Stroke()

	gc.MoveTo(0, ymax)
	gc.SetStrokeColor(color.NRGBA{255, 255, 0, 255})
	gc.FillStroke()
	for i := 0; i < xmax; i++ {
		x := float64(i)
		y := ymax - ((v.mean_long[i]-v.min)*scale + median)

		gc.LineTo(x, y)
		gc.MoveTo(x, y)
	}
	gc.Stroke()

	amount := 0.0
	account := 1000.0
	stop_loss := 0.0

	// transaction cost is 0.1%
	trans_cost := 1.0 - 0.1/100.0

	for i := 1; i < xmax; i++ {
		x := float64(i)

		prev_mean_short := v.mean_short[i-1]
		prev_mean_long := v.mean_long[i-1]

		price := v.price[i]
		mean_short := v.mean_short[i]
		mean_long := v.mean_long[i]

		buy := false
		sell := false
		if prev_mean_short < prev_mean_long && mean_short > mean_long {
			buy = true
		}
		if prev_mean_short > prev_mean_long && mean_short < mean_long {
			sell = true
		}

		if price <= stop_loss {
			sell = true
		}

		y := ymax - ((price-v.min)*scale + median)

		if buy {
			if account != 0 {
				gc.SetStrokeColor(color.NRGBA{0, 0, 255, 255})
				gc.FillStroke()
				gc.ArcTo(x, y, 3.0, 3.0, 0, 2*math.Pi)
				gc.Stroke()

				amount = account * trans_cost / price
				account = 0
				stop_loss = price * 0.9
			}
		}

		if sell {
			if amount != 0 {
				gc.SetStrokeColor(color.NRGBA{0, 255, 0, 255})
				gc.FillStroke()
				gc.ArcTo(x, y, 3.0, 3.0, 0, 2*math.Pi)
				gc.Stroke()

				account = amount * trans_cost * price
				stop_loss = 0
				amount = 0
			}
			fmt.Printf("balance: %f\n", account)
		}
	}
	gc.Stroke()

	gc.Fill()
	saveToPngFile(path, img)
}

func main() {
	file := flag.String("filename", "", "input CSV file")
	delim := flag.String("delimiter", ",", "CSV delimiter")
	flag.Parse()

	if *file == "" {
		log.Fatal("You must provide input filename")
	}

	file_reader, err := os.Open(*file)
	if err != nil {
		log.Fatal(err)
	}

	reader := csv.NewReader(file_reader)
	reader.Comma = (rune)((*delim)[0])

	line := 0

	v := &tset{window_short: 3, window_long: 17, min: 1000000000, max: 0}

	for {
		data, err := reader.Read()
		if err != nil {
			log.Print("csv reader error: %s", err)
			break
		}

		line++
		if line <= 1 {
			continue
		}

		close_price, err := strconv.ParseFloat(data[7], 64)
		if err != nil {
			log.Print(err)
			continue
		}

		v.add_price(close_price)
	}

	v.draw("/tmp/test.png")
}
