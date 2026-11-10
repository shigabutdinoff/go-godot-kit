package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gordonklaus/ineffassign/pkg/ineffassign"
	"github.com/kisielk/errcheck/errcheck"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/appends"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/atomicalign"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/cgocall"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/deepequalerrors"
	"golang.org/x/tools/go/analysis/passes/defers"
	"golang.org/x/tools/go/analysis/passes/directive"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/framepointer"
	"golang.org/x/tools/go/analysis/passes/gofix"
	"golang.org/x/tools/go/analysis/passes/hostport"
	"golang.org/x/tools/go/analysis/passes/httpmux"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"
	"golang.org/x/tools/go/analysis/passes/inline"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/modernize"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/reflectvaluecompare"
	"golang.org/x/tools/go/analysis/passes/scannererr"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/sigchanyzer"
	slogpass "golang.org/x/tools/go/analysis/passes/slog"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/sqlrowserr"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/stdversion"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/timeformat"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unsafeptr"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"golang.org/x/tools/go/analysis/passes/unusedwrite"
	"golang.org/x/tools/go/analysis/passes/waitgroup"

	"github.com/shigabutdinoff/go-godot-kit/cmd/staticlint/internal/checks"
	"github.com/shigabutdinoff/go-godot-kit/internal/buildinfo"
	"github.com/shigabutdinoff/go-godot-kit/internal/logger"
)

//go:generate go run ./gendoc

// Build version, date and commit, set at build time:
// go build -ldflags "-X main.buildVersion=v1.2.3 -X main.buildDate=2026-01-02 -X main.buildCommit=abc1234".
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

// configName is the staticcheck config next to the executable that replaces the built-in one.
const configName = "config.json"

// main prints the build version and runs multichecker with the analyzers; on a config error it logs it
// and exits with code 1. The version is not printed when go vet runs staticlint as -vettool or with -json:
// stdout carries machine-readable output then.
func main() {
	if !machineOutput(os.Args[1:]) {
		if err := buildinfo.Write(os.Stdout, buildVersion, buildDate, buildCommit); err != nil {
			l := logger.New(os.Stderr, "staticlint", logger.ModuleVersion(), slog.LevelInfo).With("exception.message", err)
			slog.NewLogLogger(l.Handler(), slog.LevelError).Fatal("version not printed")
		}
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "" // no executable path: use the built-in config
	}
	as, err := analyzers(exe)
	if err != nil {
		l := logger.New(os.Stderr, "staticlint", logger.ModuleVersion(), slog.LevelInfo).With("exception.message", err)
		slog.NewLogLogger(l.Handler(), slog.LevelError).Fatal("config not loaded")
	}
	multichecker.Main(as...)
}

// machineOutput reports whether args ask for output that programs parse: the go vet -vettool protocol
// (-flags, -V=full, a .cfg file) or -json.
func machineOutput(args []string) bool {
	for _, a := range args {
		if a == "-flags" || strings.HasPrefix(a, "-V") || strings.HasSuffix(a, ".cfg") || strings.HasPrefix(a, "-json") {
			return true
		}
	}
	return false
}

// config returns configName next to exe, or the built-in config if exe is empty or there is no such file.
func config(exe string) ([]byte, error) {
	if exe == "" {
		return checks.Default, nil
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), configName))
	if errors.Is(err, fs.ErrNotExist) {
		return checks.Default, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configName, err)
	}
	return data, nil
}

// analyzers returns the multichecker analyzers: the standard passes, modernize (without any),
// errcheck, ineffassign, OSExitAnalyzer and the staticcheck checks from the config next to exe.
func analyzers(exe string) ([]*analysis.Analyzer, error) {
	data, err := config(exe)
	if err != nil {
		return nil, err
	}
	scs, err := checks.Staticcheck(data)
	if err != nil {
		return nil, err
	}
	as := []*analysis.Analyzer{
		appends.Analyzer, asmdecl.Analyzer, assign.Analyzer, atomic.Analyzer,
		bools.Analyzer, buildtag.Analyzer, cgocall.Analyzer, composite.Analyzer,
		copylock.Analyzer, defers.Analyzer, directive.Analyzer, errorsas.Analyzer,
		framepointer.Analyzer, hostport.Analyzer, httpresponse.Analyzer, ifaceassert.Analyzer,
		loopclosure.Analyzer, lostcancel.Analyzer, nilfunc.Analyzer, nilness.Analyzer,
		printf.Analyzer, shadow.Analyzer, shift.Analyzer, sigchanyzer.Analyzer,
		slogpass.Analyzer, stdmethods.Analyzer, stdversion.Analyzer, stringintconv.Analyzer,
		structtag.Analyzer, testinggoroutine.Analyzer, tests.Analyzer, timeformat.Analyzer,
		unmarshal.Analyzer, unreachable.Analyzer, unsafeptr.Analyzer, unusedresult.Analyzer,
		unusedwrite.Analyzer, waitgroup.Analyzer,

		atomicalign.Analyzer, deepequalerrors.Analyzer, gofix.Analyzer, httpmux.Analyzer,
		inline.Analyzer, reflectvaluecompare.Analyzer, scannererr.Analyzer, sortslice.Analyzer,
		sqlrowserr.Analyzer,
		errcheck.Analyzer, ineffassign.Analyzer,
		OSExitAnalyzer,
	}
	for _, a := range modernize.Suite {
		if a != modernize.AnyAnalyzer {
			as = append(as, a)
		}
	}
	for _, a := range scs {
		as = append(as, a.Analyzer)
	}
	return as, nil
}
