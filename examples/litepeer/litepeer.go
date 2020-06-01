package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/StreamSpace/ss-light-client/lib"
	logger "github.com/ipfs/go-log/v2"
)

// Command arguments
var (
	repo        = flag.String("repo", ".", "Path for storing intermediate data")
	destination = flag.String("dst", ".", "Path to store downloaded file")
	sharable    = flag.String("sharable", "", "Sharable string provided for file")
	timeout     = flag.String("timeout", "15m", "Timeout duration")
	onlyInfo    = flag.Bool("info", false, "Get only fetch info")
	enableLog   = flag.Bool("logToStderr", false, "Enable app logs on stderr")
	showProg    = flag.Bool("progress", false, "Enable progress on stdout")
)

func returnError(err string, printUsage bool) {
	fmt.Println("ERR: " + err)
	if printUsage {
		fmt.Println(`
Usage:
	./ss-light <OPTIONS>

Options:
		`)
		flag.PrintDefaults()
	}
	os.Exit(1)
}

var clear map[string]func() //create a map for storing clear funcs

func init() {
	clear = make(map[string]func()) //Initialize it
	clear["linux"] = func() {
		cmd := exec.Command("clear") //Linux example, its tested
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
	clear["darwin"] = func() {
		cmd := exec.Command("clear") //Linux example, its tested
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
	clear["windows"] = func() {
		cmd := exec.Command("cmd", "/c", "cls") //Windows example, its tested
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
}

func CallClear() {
	value, ok := clear[runtime.GOOS] //runtime.GOOS -> linux, windows, darwin etc.
	if ok {                          //if we defined a clear func for that platform:
		value() //we execute it
	} else { //unsupported platform
		panic("Your platform is unsupported! I can't clear terminal screen :(")
	}
}

func main() {
	flag.Parse()

	if *enableLog && *showProg {
		returnError("Log and progress options cannot be used together", true)
	} else if *enableLog {
		logger.SetLogLevel("*", "debug")
	}
	if len(*sharable) == 0 {
		returnError("Sharable string not provided", true)
	}
	lc, err := lib.NewLightClient(*destination, *repo, *timeout)
	if err != nil {
		returnError("Failed setting up client reason:"+err.Error(), true)
	}
	var upd lib.ProgressUpdater
	upd = &noopProgress{}
	if !*onlyInfo && *showProg {
		upd = &updateProgress{}
	}
	status, err := lc.Start(*sharable, *onlyInfo, upd)
	if err != nil {
		returnError(status+" reason:"+err.Error(), false)
	}
	fmt.Println(status)
	return
}

type noopProgress struct{}

func (u *noopProgress) UpdateProgress(p int) {
	return
}

type updateProgress struct {
	started bool
}

func (u *updateProgress) UpdateProgress(p int) {
	if u.started {
		CallClear()
	} else {
		u.started = true
	}
	fmt.Printf("Progress %d%%\n", p)
}
