package checkprocs

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/jessevdk/go-flags"
	"github.com/mackerelio/checkers"
)

// https://github.com/sensu-plugins/sensu-plugins-process-checks
var opts struct {
	WarningOver   *int64   `short:"w" long:"warning-over" value-name:"N" description:"Trigger a warning if over a number"`
	WarnOver      *int64   `long:"warn-over" value-name:"N" description:"(DEPRECATED) Trigger a warning if over a number"`
	CritOver      *int64   `short:"c" long:"critical-over" value-name:"N" description:"Trigger a critical if over a number"`
	WarningUnder  int64    `short:"W" long:"warning-under" value-name:"N" default:"1" description:"Trigger a warning if under a number"`
	WarnUnder     int64    `long:"warn-under" value-name:"N" default:"1" description:"(DEPRECATED) Trigger a warning if under a number"`
	CritUnder     int64    `short:"C" long:"critical-under" value-name:"N" default:"1" description:"Trigger a critial if under a number"`
	MatchSelf     bool     `short:"m" long:"match-self" description:"Match itself"`
	MatchParent   bool     `short:"M" long:"match-parent" description:"Match parent"`
	CmdPatterns   []string `short:"p" long:"pattern" value-name:"PATTERN" description:"Match a command against these patterns"`
	CmdExcludePat string   `short:"x" long:"exclude-pattern" value-name:"PATTERN" description:"Don't match against a pattern to prevent false positives"`
	Ppid          string   `long:"ppid" value-name:"PPID" description:"Check against a specific PPID"`
	FilePid       string   `short:"f" long:"file-pid" value-name:"PID" description:"Check against a specific PID"`
	Vsz           int64    `short:"z" long:"virtual-memory-size" value-name:"VSZ" description:"Trigger on a Virtual Memory size is bigger than this"`
	Rss           int64    `short:"r" long:"resident-set-size" value-name:"RSS" description:"Trigger on a Resident Set size is bigger than this"`
	Pcpu          float64  `short:"P" long:"proportional-set-size" value-name:"PCPU" description:"Trigger on a Proportional Set Size is bigger than this"`
	Thcount       int64    `short:"T" long:"thread-count" value-name:"THCOUNT" description:"Trigger on a Thread Count is bigger than this"`
	State         string   `short:"s" long:"state" value-name:"STATE" description:"Trigger on a specific state, example: Z for zombie"`
	User          string   `short:"u" long:"user" value-name:"USER" description:"Trigger on a specific user"`
	Usernot       string   `short:"U" long:"user-not" value-name:"USER" description:"Trigger if not owned a specific user"`
	EsecOver      int64    `short:"e" long:"esec-over" value-name:"SECONDS" description:"Match processes that older that this, in SECONDS"`
	EsecUnder     int64    `short:"E" long:"esec-under" value-name:"SECONDS" description:"Match process that are younger than this, in SECONDS"`
	CPUOver       int64    `short:"i" long:"cpu-over" value-name:"SECONDS" description:"Match processes cpu time that is older than this, in SECONDS"`
	CPUUnder      int64    `short:"I" long:"cpu-under" value-name:"SECONDS" description:"Match processes cpu time that is younger than this, in SECONDS"`
}

type procState struct {
	cmd     string
	user    string
	ppid    string
	pid     string
	vsz     int64
	rss     int64
	pcpu    float64
	thcount int64
	state   string
	esec    int64
	csec    int64
}

// Do the plugin
func Do() {
	ckr := run(os.Args[1:])
	ckr.Name = "Procs"
	ckr.Exit()
}

func run(args []string) *checkers.Checker {
	_, err := flags.ParseArgs(&opts, args)
	if err != nil {
		os.Exit(1)
	}

	// for backward compatibility
	if opts.WarnUnder != 1 && opts.WarningUnder == 1 {
		opts.WarningUnder = opts.WarnUnder
	}
	if opts.WarnOver != nil && opts.WarningOver == nil {
		opts.WarningOver = opts.WarnOver
	}

	procs, err := getProcs()
	if err != nil {
		return checkers.NewChecker(checkers.UNKNOWN, err.Error())
	}
	var cmdPatRegexp []*regexp.Regexp
	for _, ptn := range opts.CmdPatterns {
		r, err := regexp.Compile(ptn)
		if err != nil {
			return checkers.NewChecker(checkers.UNKNOWN, err.Error())
		}
		cmdPatRegexp = append(cmdPatRegexp, r)
	}
	if len(cmdPatRegexp) == 0 {
		cmdPatRegexp = append(cmdPatRegexp, regexp.MustCompile(".*"))
	}
	cmdExcludePatRegexp := regexp.MustCompile(".*")
	if opts.CmdExcludePat != "" {
		r, err := regexp.Compile(opts.CmdExcludePat)
		if err != nil {
			return checkers.NewChecker(checkers.UNKNOWN, err.Error())
		}
		cmdExcludePatRegexp = r
	}
	result := checkers.OK
	var msg string

	var resultrocStates []procState
	for _, reg := range cmdPatRegexp {
		for _, proc := range procs {
			if matchProc(proc, reg, cmdExcludePatRegexp) {
				resultrocStates = append(resultrocStates, proc)
			}
		}
		count := int64(len(resultrocStates))
		result = mergeStatus(count, result)
		msg += fmt.Sprintf("\n%s", gatherMsg(count, reg.String()))

		resultrocStates = []procState{}
	}
	return checkers.NewChecker(result, msg)
}

func matchProc(proc procState, cmdPatRegexp *regexp.Regexp, cmdExcludePatRegexp *regexp.Regexp) bool {
	return (len(opts.CmdPatterns) == 0 || cmdPatRegexp.MatchString(proc.cmd)) &&
		(opts.CmdExcludePat == "" || !cmdExcludePatRegexp.MatchString(proc.cmd)) &&
		(opts.MatchSelf || proc.pid != strconv.Itoa(os.Getpid())) &&
		(opts.MatchParent || proc.pid != strconv.Itoa(os.Getppid())) &&
		(opts.Ppid == "" || proc.ppid == opts.Ppid) &&
		(opts.FilePid == "" || proc.pid == opts.FilePid) &&
		(opts.Vsz == 0 || proc.vsz <= opts.Vsz) &&
		(opts.Rss == 0 || proc.rss <= opts.Rss) &&
		(opts.Pcpu == 0 || proc.pcpu <= opts.Pcpu) &&
		(opts.Thcount == 0 || proc.thcount <= opts.Thcount) &&
		(opts.State == "" || proc.state == opts.State) &&
		(opts.User == "" || proc.user == opts.User) &&
		(opts.Usernot == "" || proc.user != opts.Usernot) &&
		(opts.EsecUnder == 0 || proc.esec < opts.EsecUnder) &&
		(opts.EsecOver == 0 || proc.esec > opts.EsecOver) &&
		(opts.CPUUnder == 0 || proc.csec < opts.CPUUnder) &&
		(opts.CPUOver == 0 || proc.csec > opts.CPUOver)
}

func gatherMsg(count int64, pattern string) string {
	msg := fmt.Sprintf("Found %d matching processes", count)
	if len(opts.CmdPatterns) != 0 {
		msg += fmt.Sprintf("; cmd /%s/", pattern)
	}
	if opts.State != "" {
		msg += fmt.Sprintf("; state /%s/", opts.State)
	}
	if opts.User != "" {
		msg += fmt.Sprintf("; user /%s/", opts.User)
	}
	if opts.Usernot != "" {
		msg += fmt.Sprintf("; usernot /%s/", opts.Usernot)
	}
	if opts.Vsz != 0 {
		msg += fmt.Sprintf("; vsz < %d", opts.Vsz)
	}
	if opts.Rss != 0 {
		msg += fmt.Sprintf("; rss < %d", opts.Rss)
	}
	if opts.Pcpu != 0 {
		msg += fmt.Sprintf("; pcpu < %f", opts.Pcpu)
	}
	if opts.Thcount != 0 {
		msg += fmt.Sprintf("; thcount < %d", opts.Thcount)
	}
	if opts.EsecUnder != 0 {
		msg += fmt.Sprintf("; esec < %d", opts.EsecUnder)
	}
	if opts.EsecOver != 0 {
		msg += fmt.Sprintf("; esec > %d", opts.EsecOver)
	}
	if opts.CPUUnder != 0 {
		msg += fmt.Sprintf("; csec < %d", opts.CPUUnder)
	}
	if opts.CPUOver != 0 {
		msg += fmt.Sprintf("; csec > %d", opts.CPUOver)
	}
	if opts.Ppid != "" {
		msg += fmt.Sprintf("; ppid %s", opts.Ppid)
	}
	if opts.FilePid != "" {
		msg += fmt.Sprintf("; pid %s", opts.FilePid)
	}
	return msg
}

func mergeStatus(count int64, current checkers.Status) checkers.Status {
	result := checkers.OK
	if opts.CritUnder != 0 && count < opts.CritUnder ||
		opts.CritOver != nil && count > *opts.CritOver {
		result = checkers.CRITICAL
	} else if opts.WarningUnder != 0 && count < opts.WarningUnder ||
		opts.WarningOver != nil && count > *opts.WarningOver {
		result = checkers.WARNING
	}

	if result > checkers.WARNING {
		return checkers.CRITICAL
	} else if result > checkers.OK {
		return checkers.WARNING
	} else {
		return checkers.OK
	}
}
