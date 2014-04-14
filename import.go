package main

import (
	"bufio"
	"code.google.com/p/draw2d/draw2d"
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/datastream/skyline"
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
	ymax, yscale float64
	xmax int
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

func (v *tset) trend(start int, price []float64) (float64, float64, float64) {
	num := len(price)

	var points []skyline.TimePoint = make([]skyline.TimePoint, num, num)

	for j := 0; j < num; j++ {
		points[j].Timestamp = int64(start + j)
		points[j].Value = price[j]
	}

	m, c := skyline.LinearRegressionLSE(points)

	sum := 0.0
	for _, val := range points {
		projected := m*float64(val.Timestamp) + c
		sum += math.Pow(val.Value-projected, 2)
	}

	return m, c, math.Sqrt(sum) / float64(num)
}

func (v *tset) scale(y float64) float64 {
	return v.ymax - (y - v.min) * v.yscale
}

func (v *tset) draw_trend(gc draw2d.GraphicContext, start, stop int, m, c float64, color color.Color) {
	gc.SetStrokeColor(color)

	x0 := float64(start)
	y0 := m * x0 + c
	y0 = v.scale(y0)

	x1 := float64(stop)
	y1 := m * x1 + c
	y1 = v.scale(y1)

	gc.MoveTo(x0, y0)
	gc.LineTo(x1, y1)
	gc.Stroke()
}

func (v *tset) draw_grid(gc draw2d.GraphicContext) {
	gc.SetStrokeColor(color.NRGBA{255, 0, 0, 100})
	for i := 0; i < v.xmax; i += int(v.window_short) {
		x := float64(i)
		gc.MoveTo(x, 0)
		gc.LineTo(x, v.ymax)
		gc.Stroke()
	}

	gc.SetStrokeColor(color.NRGBA{120, 120, 0, 100})
	for i := 0; i < v.xmax; i += int(v.window_long) {
		x := float64(i)
		gc.MoveTo(x, 0)
		gc.LineTo(x, v.ymax)
		gc.Stroke()
	}
}

func (v *tset) draw(path string) {
	img := image.NewRGBA(image.Rect(0, 0, len(v.price), 2000))
	gc := draw2d.NewGraphicContext(img)
	gc.Clear()
	gc.SetStrokeColor(image.Black)
	gc.SetFillColor(image.White)
	gc.SetLineWidth(1)
	gc.SetLineCap(draw2d.ButtCap)
	gc.FillStroke()

	v.ymax = float64(img.Bounds().Max.Y)
	v.yscale = v.ymax / (v.max - v.min)

	fmt.Printf("scale: %f, min: %f, max: %f\n", v.yscale, v.min, v.max)
	v.xmax = len(v.price)
	if v.xmax > img.Bounds().Max.X {
		v.xmax = img.Bounds().Max.X
	}

	//v.draw_grid(gc)

	gc.MoveTo(0, v.ymax)
	for i := 0; i < v.xmax; i++ {
		x := float64(i)
		y := v.scale(v.price[i])

		gc.LineTo(x, y)
		gc.MoveTo(x, y)
	}
	gc.Stroke()

	gc.MoveTo(0, v.ymax)
	gc.SetStrokeColor(color.NRGBA{255, 0, 0, 255})
	for i := 0; i < v.xmax; i++ {
		x := float64(i)
		y := v.scale(v.mean_short[i])

		gc.LineTo(x, y)
		gc.MoveTo(x, y)
	}
	gc.Stroke()

	gc.MoveTo(0, v.ymax)
	gc.SetStrokeColor(color.NRGBA{120, 120, 0, 255})
	for i := 0; i < v.xmax; i++ {
		x := float64(i)
		y := v.scale(v.mean_long[i])

		gc.LineTo(x, y)
		gc.MoveTo(x, y)
	}
	gc.Stroke()

	amount := 0.0
	account := 1000.0
	prev_account := 0.0

	stop_loss := 0.0
	profit_fix := 0.0

	// transaction cost is 0.1%
	trans_cost := 1.0 - 0.1/100.0

	for i := 1; i < v.xmax; i++ {
		prev_mean_short := v.mean_short[i-1]
		prev_mean_long := v.mean_long[i-1]

		price := v.price[i]
		mean_short := v.mean_short[i]
		mean_long := v.mean_long[i]

		buy := false
		sell := false
		if prev_mean_short < prev_mean_long && mean_short > mean_long && account != 0 {
			buy = true
		}
		if prev_mean_short > prev_mean_long && mean_short < mean_long {
			//sell = true
		}

		if price <= stop_loss && amount != 0 {
			sell = true
		}
		if price > profit_fix && amount != 0 {
			sell = true
		}

		x := float64(i)
		y := v.scale(price)

		big_num := 25
		num := 5
		if i > big_num {
			start := i - num
			stop := i
			//m, c, _ := v.trend(start, v.price[start : stop])

			start = i - big_num
			stop = i
			ml, cl, stdl := v.trend(start, v.price[start : stop])

			var op uint8 = 0
			if stdl < 0.01 {
				op = 255
			}
			v.draw_trend(gc, start, stop, ml, cl, color.NRGBA{0, 0, 255, op})
			//v.draw_trend(gc, start, stop, ml, cl, color.NRGBA{0, 120, 255, op})

			fmt.Printf("%d/%d: price: %f, m: %f, c: %f, std: %f\n", i, v.xmax, price, ml, cl, stdl)
			//v.draw_trend(gc, start, stop, m, c, color.NRGBA{255, 0, 0, 100})
		}

		if buy {
			if account != 0 {
				gc.SetStrokeColor(color.NRGBA{0, 0, 255, 255})
				gc.FillStroke()
				gc.ArcTo(x, y, 3.0, 3.0, 0, 2*math.Pi)
				gc.Stroke()

				fmt.Printf(" buy balance: %f\n", account)
				amount = account * trans_cost / price
				prev_account = account
				account = 0
				stop_loss = price * 0.995
				profit_fix = price * 1.02
			}
		}

		if sell {
			if amount != 0 {
				gc.ArcTo(x, y, 3.0, 3.0, 0, 2*math.Pi)
				gc.SetStrokeColor(color.NRGBA{0, 255, 0, 255})
				gc.SetFillColor(color.NRGBA{0, 255, 0, 255})

				if price < stop_loss {
					gc.FillStroke()
				} else {
					gc.Stroke()
				}

				account = amount * trans_cost * price
				fmt.Printf("sell balance: %f %.2f%%\n", account, (account - prev_account) / prev_account * 100.0)
				stop_loss = 0
				amount = 0
			}
		}
	}
	gc.Stroke()

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

	v := &tset{window_short: 5, window_long: 20, min: 1000000000, max: 0}

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
