/*
Example code showing how to perform object detection using a YOLOv5 model.
*/
package main

import (
	"flag"
	"fmt"
	"github.com/swdee/go-rknnlite"
	"github.com/swdee/go-rknnlite/postprocess"
	"gocv.io/x/gocv"
	"image"
	"image/color"
	"log"
	"time"
)

func main() {
	// disable logging timestamps
	log.SetFlags(0)

	// read in cli flags
	modelFile := flag.String("m", "../data/yolov5s-640-640-rk3588.rknn", "RKNN compiled YOLO model file")
	imgFile := flag.String("i", "../data/bus.jpg", "Image file to run object detection on")
	labelFile := flag.String("l", "../data/coco_80_labels_list.txt", "Text file containing model labels")
	saveFile := flag.String("o", "../data/bus-yolov5-out.jpg", "The output JPG file with object detection markers")

	flag.Parse()

	err := rknnlite.SetCPUAffinity(rknnlite.RK3588FastCores)

	if err != nil {
		log.Printf("Failed to set CPU Affinity: %w", err)
	}

	// create rknn runtime instance
	rt, err := rknnlite.NewRuntime(*modelFile, rknnlite.NPUCoreAuto)

	if err != nil {
		log.Fatal("Error initializing RKNN runtime: ", err)
	}

	// set runtime to leave output tensors as int8
	rt.SetWantFloat(false)

	// optional querying of model file tensors and SDK version.  not necessary
	// for production inference code
	inputAttrs := optionalQueries(rt)

	// create YOLOv5 post processor
	yoloProcesser := postprocess.NewYOLOv5(postprocess.YOLOv5COCOParams())

	// load in Model class names
	classNames, err := rknnlite.LoadLabels(*labelFile)

	if err != nil {
		log.Fatal("Error loading model labels: ", err)
	}

	// load image
	img := gocv.IMRead(*imgFile, gocv.IMReadColor)

	if img.Empty() {
		log.Fatal("Error reading image from: ", *imgFile)
	}

	// convert colorspace and resize image
	rgbImg := gocv.NewMat()
	gocv.CvtColor(img, &rgbImg, gocv.ColorBGRToRGB)

	cropImg := rgbImg.Clone()
	scaleSize := image.Pt(int(inputAttrs[0].Dims[1]), int(inputAttrs[0].Dims[2]))
	gocv.Resize(rgbImg, &cropImg, scaleSize, 0, 0, gocv.InterpolationArea)

	defer img.Close()
	defer rgbImg.Close()
	defer cropImg.Close()

	start := time.Now()

	// perform inference on image file
	outputs, err := rt.Inference([]gocv.Mat{cropImg})

	if err != nil {
		log.Fatal("Runtime inferencing failed with error: ", err)
	}

	endInference := time.Now()

	log.Println("outputs=", len(outputs.Output))

	detectResults := yoloProcesser.DetectObjects(outputs)

	endDetect := time.Now()

	log.Printf("Model first run speed: inference=%s, post processing=%s, total time=%s\n",
		endInference.Sub(start).String(),
		endDetect.Sub(endInference).String(),
		endDetect.Sub(start).String(),
	)

	for _, detResult := range detectResults {

		text := fmt.Sprintf("%s %.1f%%", classNames[detResult.Class], detResult.Probability*100)
		fmt.Printf("%s @ (%d %d %d %d) %f\n", classNames[detResult.Class], detResult.Box.Left, detResult.Box.Top, detResult.Box.Right, detResult.Box.Bottom, detResult.Probability)

		// Draw rectangle around detected object
		rect := image.Rect(detResult.Box.Left, detResult.Box.Top, detResult.Box.Right, detResult.Box.Bottom)
		gocv.Rectangle(&img, rect, color.RGBA{R: 0, G: 0, B: 255, A: 0}, 2)

		// Put text
		gocv.PutText(&img, text, image.Pt(detResult.Box.Left, detResult.Box.Top+12), gocv.FontHersheyDuplex, 0.4, color.RGBA{R: 255, G: 255, B: 255, A: 0}, 1)
	}

	// Save the result
	if ok := gocv.IMWrite(*saveFile, img); !ok {
		log.Fatal("Failed to save the image")
	}

	log.Printf("Saved object detection result to %s\n", *saveFile)

	// free outputs allocated in C memory after you have finished post processing
	err = outputs.Free()

	if err != nil {
		log.Fatal("Error freeing Outputs: ", err)
	}

	// optional code.  run benchmark to get average time of 10 runs
	runBenchmark(rt, yoloProcesser, []gocv.Mat{cropImg})

	// close runtime and release resources
	err = rt.Close()

	if err != nil {
		log.Fatal("Error closing RKNN runtime: ", err)
	}

	log.Println("done")
}

func runBenchmark(rt *rknnlite.Runtime, yoloProcesser *postprocess.YOLOv5,
	mats []gocv.Mat) {

	count := 100
	start := time.Now()

	for i := 0; i < count; i++ {
		// perform inference on image file
		outputs, err := rt.Inference(mats)

		if err != nil {
			log.Fatal("Runtime inferencing failed with error: ", err)
		}

		// post process
		_ = yoloProcesser.DetectObjects(outputs)

		err = outputs.Free()

		if err != nil {
			log.Fatal("Error freeing Outputs: ", err)
		}
	}

	end := time.Now()
	total := end.Sub(start)
	avg := total / time.Duration(count)

	log.Printf("Benchmark time=%s, count=%d, average total time=%s\n",
		total.String(), count, avg.String(),
	)
}

func optionalQueries(rt *rknnlite.Runtime) []rknnlite.TensorAttr {

	// get SDK version
	ver, err := rt.SDKVersion()

	if err != nil {
		log.Fatal("Error initializing RKNN runtime: ", err)
	}

	fmt.Printf("Driver Version: %s, API Version: %s\n", ver.DriverVersion, ver.APIVersion)

	// get model input and output numbers
	num, err := rt.QueryModelIONumber()

	if err != nil {
		log.Fatal("Error querying IO Numbers: ", err)
	}

	log.Printf("Model Input Number: %d, Ouput Number: %d\n", num.NumberInput, num.NumberOutput)

	// query Input tensors
	inputAttrs, err := rt.QueryInputTensors()

	if err != nil {
		log.Fatal("Error querying Input Tensors: ", err)
	}

	log.Println("Input tensors:")

	for _, attr := range inputAttrs {
		log.Printf("  %s\n", attr.String())
	}

	// query Output tensors
	outputAttrs, err := rt.QueryOutputTensors()

	if err != nil {
		log.Fatal("Error querying Output Tensors: ", err)
	}

	log.Println("Output tensors:")

	for _, attr := range outputAttrs {
		log.Printf("  %s\n", attr.String())
	}

	return inputAttrs
}
