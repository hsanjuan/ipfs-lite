package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	vs "github.com/StreamSpace/ss-light-client/examples/litepeer/version"
	"github.com/StreamSpace/ss-light-client/lib"
	logger "github.com/ipfs/go-log/v2"
)

// Command arguments
var (
	destination = flag.String("dst", ".", "Complete file path on disk to store downloaded file")
	sharable    = flag.String("sharable", "", "Sharable string provided for file")
	timeout     = flag.String("timeout", "15m", "Timeout duration for download")
	onlyInfo    = flag.Bool("info", false, "Get only fetch info")
	stat        = flag.Bool("stat", false, "Get stat of the last fetch")
	enableLog   = flag.Bool("logToStderr", false, "Enable app logs on stderr")
	showProg    = flag.Bool("progress", false, "Enable progress on stdout")
	jsonOut     = flag.Bool("json", false, "Display output in json format")
	help        = flag.Bool("help", false, "Show command usage")
	version     = flag.Bool("version", false, "Show to enviroment and commit hash")
)

func returnError(err string, printUsage bool) {
	fmt.Println("ERR: " + err)
	if printUsage {
		usage()
	}
	os.Exit(1)
}

func usage() {
	fmt.Println(`
Usage:
	./swrm-client <OPTIONS>

Options:
		`)
	flag.PrintDefaults()
	fmt.Println(`
Description:

The light-client will download a file from Hive dcdn based on the provided 
sharable link. By default the downloaded file will be in the same location 
as the light-client binary itself.

    > ./swrm-client -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX

To save the binary in a custom location with a custom name, you need to provide 
the path along with the filename in '-dst' flag.  By default the file will be 
saved where the binary is with the default filename. 

    > ./swrm-client -dst $HOME/greeter.txt -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX

To only see the link information you add the '-info' flag.

    > ./swrm-client -info -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX

By default light-client returns normal text as output. If you need a json output 
add '-json' flag with your command.

    > ./swrm-client -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX -json
  
To see the download progress use '-progress' flag.

    > ./swrm-client -dst $HOME/greeter.txt -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX -progress

To see the logs of the command use '-logToStderr' flag. Note : '-logToStderr' and 
'-progress' flags cannot be used together.

    > ./swrm-client -dst $HOME/greeter.txt -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX -logToStderr
 
To see the connected peers and ledger for the last download use '-stat' flag.

    > ./swrm-client -dst $HOME/greeter.txt -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX -stat
  
Depending on hiver nodes availability download might take some time. you can set a minimum 
timeout for the download to finish. default is 15m.
 	
    > ./swrm-client -dst $HOME/greeter.txt -sharable fzhnp4jhFnMUKVGMKpt4kBMrvX -timeout 5m

To see usage

    > ./swrm-client -help
`)
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

	if *help {
		usage()
		return
	}

	if *version {
		fmt.Printf("%s-%s", vs.Env, vs.Commit)
		fmt.Println()
		return
	}

	if *enableLog && *showProg {
		returnError("Log and progress options cannot be used together", true)
	} else if *enableLog {
		logger.SetLogLevel("*", "debug")
	}
	if len(*sharable) == 0 {
		returnError("Sharable string not provided", true)
	}
	lc, err := lib.NewLightClient(*destination, *timeout, *jsonOut)
	if err != nil {
		returnError("Failed setting up client reason:"+err.Error(), true)
	}
	var upd lib.ProgressUpdater
	upd = &noopProgress{}
	if !*onlyInfo && *showProg {
		upd = &updateProgress{
			jsonOut: *jsonOut,
		}
	}
	out := lc.Start(*sharable, *onlyInfo, *stat, upd)
	lib.OutMessage(out, *jsonOut)
	return
}

type noopProgress struct{}

func (u *noopProgress) UpdateProgress(p lib.ProgressOut) {
	return
}

type updateProgress struct {
	started bool
	jsonOut bool
}

func (u *updateProgress) UpdateProgress(p lib.ProgressOut) {
	var out *lib.Out
	if u.jsonOut {
		out = lib.NewOut(200, "Progress", "", p)
	} else {
		out = lib.NewOut(200, "Progress", "", fmt.Sprintf("%d%% (%s / %s)", p.Percentage, p.Downloaded, p.TotalSize))
	}
	lib.OutMessage(out, u.jsonOut)
}
