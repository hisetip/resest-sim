package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const numberOfExperiments = 10 //always the same

const distribution = "lognormal"

const confidenceLevelUsed = 90
const maxInterval = 0.15

const degree = 7 // the degree of the user experiment

var (
	numberOfNodes int
	safetyTTL int

	histogram map[int] int
	tDistributionTable map[string] float64

	resources []int // node -> resource
	nodesAlreadyVisited map[int] bool // node -> yes/no
)

func main() {
	var resultsToPrint []string

	for numberOfNodes = 1000; numberOfNodes <= 1000000; {
		safetyTTL = numberOfNodes - 3
		appendToPrint, resultHistogram, me := runExp()
		resultsToPrint = append(resultsToPrint, appendToPrint)
		sum := 0.0
		realHistogram := make(map[int] int)
		for i := 0; i < len(resources); i++ {
			sum += float64(resources[i])
			realHistogram[resources[i]]++
		}

		if me != -1 { //TTL not hit
			writeMapToFile(fmt.Sprintf("tmp/logs/resource-estimator-sim/%vconfLevel/%vmaxErr/%vnodes/real.histogram",
				confidenceLevelUsed, maxInterval, len(resources)), realHistogram)
			reducedRealHistogram := reduceHistogram(realHistogram, resources[me])
			normalizedRealHistogram := normalizeHistogramInt(reducedRealHistogram)

			histogramError := compareHistograms(normalizedRealHistogram, resultHistogram)

			resultsToPrint = append(resultsToPrint,fmt.Sprintf("%v nodes – Real Mean: %f\n", numberOfNodes, sum/float64(len(resources))))
			resultsToPrint = append(resultsToPrint,fmt.Sprintf("%v nodes – Histogram error: %f\n", numberOfNodes, histogramError))
			//resultsToPrint = append(resultsToPrint, fmt.Sprintf("Real Histogram: %v\n", normalizedRealHistogram))
			//resultsToPrint = append(resultsToPrint, fmt.Sprintf("Obtained Histogram: %v\n", resultHistogram))
		}

		if numberOfNodes == 50 {
			numberOfNodes = 100
		} else {
			numberOfNodes*=1000
		}
	}
	fmt.Println("-------------------------------------")
	for _, s := range resultsToPrint {
		fmt.Print(s)
	}
}

// Runs numberOfExperiments experiments
func runExp() (string, map[int]float64, int) {
	resources = readResourcesToArray()
	histogram = make(map[int] int)
	nodesAlreadyVisited = make(map[int] bool)

	myIndex := getRandInt(len(resources))
	nodesAlreadyVisited[myIndex] = true

	sumOfHops := 0
	sumOfMeans := 0.0
	var finalHistograms []map[int]int
	for n := 0; n < numberOfExperiments; n++ {

		for i := 0; ; {
			partialView := getRandInts(len(resources), degree)
			rInt := partialView[getRandInt(len(partialView))]
			if _, exists := nodesAlreadyVisited[rInt]; exists {
				continue
			}
			nodesAlreadyVisited[rInt] = true
			histogram[resources[rInt]]++

			if isIt, mean := isMarginOfErrorWithinBounds(histogram, maxInterval); isIt {
				fmt.Printf("SUCCESS, with mean %v and %v hops\n", mean, i)
				sumOfHops+=i
				sumOfMeans+=mean

				/*//testing
				hr := reduceHistogram(histogram, resources[myIndex])
				hn := normalizeHistogramInt(hr)
				fmt.Println(hn)*/
				break
			}

			i++

			if i >= safetyTTL {
				return fmt.Sprintf("TTL HIT! Had already successfully performed %v experiments. 🥲\n", n), map[int]float64{}, -1
			}
		}
		os.MkdirAll(fmt.Sprintf("tmp/logs/resource-estimator-sim/%vconfLevel/%vmaxErr/%vnodes", confidenceLevelUsed,
			maxInterval, len(resources)), os.ModePerm)
		writeMapToFile(fmt.Sprintf("tmp/logs/resource-estimator-sim/%vconfLevel/%vmaxErr/%vnodes/obtained%v.histogram",
			confidenceLevelUsed, maxInterval, len(resources), n), histogram)
		finalHistograms = append(finalHistograms, reduceHistogram(histogram, resources[myIndex]))

		//resetting
		histogram = make(map[int] int)
		nodesAlreadyVisited = make(map[int] bool)
		nodesAlreadyVisited[myIndex] = true
	}

	sumHistogram := make(map[int] int)
	for _, finalHistogram := range finalHistograms {
		for k, v := range finalHistogram {
			sumHistogram[k]+=v
		}
	}
	avgHistogram := make(map[int] float64)
	for k, v := range sumHistogram {
		avgHistogram[k] = float64(v)/float64(len(finalHistograms))
	}

	normalizedAvgHistogram := normalizeHistogram(avgHistogram)

	return fmt.Sprintf("\n%v nodes – Average number of hops: %v, avg means: %f\n", numberOfNodes, sumOfHops/numberOfExperiments, sumOfMeans/float64(numberOfExperiments)), normalizedAvgHistogram, myIndex
}

func compareHistograms(realHistogram map[int]float64, obtainedHistogram map[int]float64) float64 {
	sumErrors := 0.0
	for k, v := range realHistogram {
		fmt.Println(math.Abs(v - obtainedHistogram[k]))
		sumErrors += math.Abs(v - obtainedHistogram[k])
	}

	return sumErrors/2.0
}

func normalizeHistogram(histogram map[int]float64) map[int] float64 {
	newNormalizedHistogram := make(map[int]float64)
	totalSamples := 0.0
	for _, v := range histogram {
		totalSamples+=v
	}
	for k, v := range histogram {
		newNormalizedHistogram[k] = v / totalSamples
	}

	return newNormalizedHistogram
}

func normalizeHistogramInt(histogram map[int]int) map[int] float64 {
	newNormalizedHistogram := make(map[int]float64)
	totalSamples := 0
	for _, v := range histogram {
		totalSamples+=v
	}
	for k, v := range histogram {
		newNormalizedHistogram[k] = float64(v) / float64(totalSamples)
	}

	return newNormalizedHistogram
}

func reduceHistogram(h map[int]int, myResource int) map[int] int{
	return reduceHistogram5class(h, myResource)
}

func reduceHistogram5class(h map[int]int, myResource int) map[int] int{
	newReducedHistogram := make(map[int] int)
	for k, v := range h {
		switch {
		case k < 0:
			panic("Resource level is negative...")
		case k < myResource - int(float64(myResource)*0.5):
			newReducedHistogram[0] += v
			break
		case k < myResource - int(float64(myResource)*0.1):
			newReducedHistogram[myResource - int(float64(myResource)*0.5)] += v
			break
		case k < myResource + int(float64(myResource)*0.1):
			newReducedHistogram[myResource - int(float64(myResource)*0.1)] += v
			break
		case k < myResource + int(float64(myResource)*0.5):
			newReducedHistogram[myResource + int(float64(myResource)*0.1)] += v
			break
		default:
			newReducedHistogram[myResource + int(float64(myResource)*0.5)] += v
			break
		}
	}

	return newReducedHistogram
}

func reduceHistogram3class(h map[int]int, myResource int) map[int] int{
	newReducedHistogram := make(map[int] int)
	for k, v := range h {
		switch {
		case k < 0:
			panic("Resource level is negative...")
		case k < myResource - int(float64(myResource)*0.1):
			newReducedHistogram[0] += v
			break
		case k < myResource + int(float64(myResource)*0.1):
			newReducedHistogram[myResource - int(float64(myResource)*0.1)] += v
			break
		default:
			newReducedHistogram[myResource + int(float64(myResource)*0.1)] += v
			break
			/*case k < myResource * 2:
				newReducedHistogram[myResource + int(float64(myResource)*0.5)] = v
				break
			default:
				newReducedHistogram[myResource * 2] = v
				break*/
		}
	}

	return newReducedHistogram
}

func reduceHistogram2class(h map[int]int, myResource int) map[int] int{
	newReducedHistogram := make(map[int] int)
	for k, v := range h {
		switch {
		case k < 0:
			panic("Resource level is negative...")
		case k < myResource:
			newReducedHistogram[0]+=v
			break
		default:
			newReducedHistogram[myResource]+=v
			break
		}
	}

	return newReducedHistogram
}

func readResourcesToArray() []int {
	f, err := os.Open("resources_" + distribution)
	if err != nil {
		panic(err)
	}

	var res []int

	sc := bufio.NewScanner(f)
	i := 0
	for sc.Scan() {
		r, err := strconv.Atoi(sc.Text())
		if err != nil {
			panic(err)
		}
		res = append(res, r)
		i++
		if i >= numberOfNodes {
			break
		}
	}
	//fmt.Println(res)
	return res
}

/* Returns a bool which is true if the confidence interval is low enough (relative to maxInterval variable);
also returns the computed mean.
More info: https://www.dummies.com/education/math/statistics/using-the-t-distribution-to-calculate-confidence-intervals/*/
func isMarginOfErrorWithinBounds(histogram map[int] int, maxInterval float64) (bool, float64) {
	valueSum := 0
	n := 0
	for key, element := range histogram {
		valueSum += key * element
		n += element
	}
	mean := float64(valueSum) / float64(n)

	if n <= 10  {
		return false, mean
	}

	sumDif := 0.0
	for key, element := range histogram {
		for i := 0; i < element; i++ {
			x := math.Pow(float64(key) - mean,2)
			sumDif += x
		}
	}
	//fmt.Printf("--------isMarginOfErrorWithinBounds-------------\n")
	//fmt.Printf("sumDif: %f, mean: %f\n", sumDif, mean)
	standardDeviation := math.Sqrt(sumDif/float64(n))
	//fmt.Printf("Standard deviation - %f, n: %f\n", standardDeviation, float64(n))
	marginOfError := getTValue(n) * (standardDeviation / math.Sqrt(float64(n)))

	marginOfErrorRatio := marginOfError / mean

	//fmt.Printf("Margin of error - %f, margin of error ratio: %f\n", marginOfError, marginOfErrorRatio)
	//fmt.Printf("---------------------------------------\n")

	if marginOfErrorRatio <= maxInterval {
		return true, mean
	} else {
		return false, mean
	}
}

/* Returns t-distribution (or z, if n > 30) value. */
func getTValue(n int) float64 {
	if tDistributionTable == nil {
		tDistributionTable = make(map[string] float64)
		f, err := os.Open("t-distribution-tables/t-distribution-table-" + strconv.Itoa(confidenceLevelUsed) + ".txt")
		if err != nil {
			panic(err)
		}

		for i := 1; i <= 30; i++ {
			_, err := f.Seek(0, 0)
			if err != nil {
				panic(err)
			}
			line, _, err := readLine(f, i)
			if err != nil {
				panic(err)
			}
			tDistributionTable[strconv.Itoa(i)], err = strconv.ParseFloat(line, 64)
			if err != nil {
				panic(err)
			}
		}

		_, err = f.Seek(0, 0)
		if err != nil {
			panic(err)
		}
		line, _, err := readLine(f, 31)
		if err != nil {
			panic(err)
		}
		tDistributionTable["z"], err = strconv.ParseFloat(line, 64)
	}

	if n > 0 && n <= 30 {
		return tDistributionTable[strconv.Itoa(n)]
	} else if n > 30 {
		return tDistributionTable["z"]
	} else {
		panic("Trying to get t-distribution value for negative number or 0!")
	}
}

// Reads a line from a file.
func readLine(r io.Reader, lineNum int) (line string, lastLine int, err error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lastLine++
		if lastLine == lineNum {
			// you can return sc.Bytes() if you need output in []bytes
			return sc.Text(), lastLine, sc.Err()
		}
	}
	return line, lastLine, io.EOF
}

func writeMapToFile(filename string, m map[int]int) {
	//Encoding to byte array
	b := new(bytes.Buffer)
	e := gob.NewEncoder(b)
	err := e.Encode(m)
	if err != nil {
		panic(err)
	}

	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}(file)

	_, err = file.Write(b.Bytes())
	if err != nil {
		panic(err)
	}

	err = file.Sync()
	if err != nil {
		panic(err)
	}
}

func getRandInt(roof int) int {
	rand.Seed(int64(time.Now().Nanosecond()))
	return rand.Intn(roof)
}

func getRandInts(roof int, degree int) []int {
	var rands []int

	for ;len(rands) < degree; {
		rand.Seed(int64(time.Now().Nanosecond()))
		toAdd := rand.Intn(roof)
		if contains(rands, toAdd) {
			continue
		}
		rands = append(rands, toAdd)
	}

	return rands
}

func contains(s []int, in int) bool {
	for _, v := range s {
		if v == in {
			return true
		}
	}

	return false
}