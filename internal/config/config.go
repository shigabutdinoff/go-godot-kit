// Package config parses game settings from flags, environment variables with a game prefix and a JSON file.
// Precedence: env > flag > file > default.
package config

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/shigabutdinoff/go-godot-kit/internal/errwriter"
)

// ErrUsage wraps errors in arguments, env and the config file; it selects exit code 2.
var ErrUsage = errors.New("invalid arguments")

// Config holds the resolved run settings.
type Config struct {
	Version    bool       // print the version and exit
	Project    string     // Godot project directory
	LogLevel   slog.Level // minimum log level
	LogFile    string     // log file; empty means stderr
	CPUProfile string     // CPU profile file
	MemProfile string     // memory profile file
	Godot      []string   // Godot arguments after --
}

// fileConfig is the content of the JSON config file; the log level is stored as a string.
type fileConfig struct {
	Project    string   `json:"project"`
	LogLevel   string   `json:"log_level"`
	LogFile    string   `json:"log_file"`
	CPUProfile string   `json:"cpu_profile"`
	MemProfile string   `json:"mem_profile"`
	Godot      []string `json:"godot_args"` // used when the command line has no Godot arguments after --
}

// Parse parses args (without the program name), env via lookupEnv and the file from -c, -config, CONFIG or <envPrefix>_CONFIG
// (each later one takes precedence).
// name is the program name in help; envPrefix prefixes env variables, e.g. GAME gives GAME_PROJECT.
// It writes flag help to out. Input errors are wrapped in ErrUsage; -h returns flag.ErrHelp.
func Parse(
	name, envPrefix string,
	args []string,
	lookupEnv func(string) (string, bool),
	out io.Writer,
) (Config, error) {
	var cfg Config
	var short, long string
	flags, usageErr := newFlagSet(name, envPrefix, &cfg, &short, &long, out)
	if err := parseFlags(flags, args, usageErr); err != nil {
		return cfg, err
	}

	if n := len(args) - flags.NArg(); flags.NArg() > 0 && (n == 0 || args[n-1] != "--") {
		return cfg, fmt.Errorf("%w: unexpected argument %q (Godot arguments go after --)", ErrUsage, flags.Arg(0))
	}
	if flags.NArg() > 0 {
		cfg.Godot = flags.Args()
	}
	if short != "" && long != "" && short != long {
		return cfg, fmt.Errorf("%w: -c %q and -config %q are different config files", ErrUsage, short, long)
	}
	file := cmp.Or(short, long)
	for _, env := range []string{"CONFIG", envPrefix + "_CONFIG"} {
		if v, ok := lookupEnv(env); ok && v != "" {
			file = v
		}
	}

	var fileErr error
	if file != "" {
		set := map[string]bool{}
		flags.Visit(func(f *flag.Flag) { set[f.Name] = true })
		fileErr = cfg.applyFile(file, set)
	}
	if err := errors.Join(fileErr, cfg.applyEnv(envPrefix, lookupEnv)); err != nil {
		return cfg, fmt.Errorf("%w: %w", ErrUsage, err)
	}
	return cfg, nil
}

// newFlagSet returns the flag set of program name that stores values in cfg and the -c and -config paths
// in short and long. The set prints help, naming envPrefix variables, to out; the second function returns
// the help write error, if any.
func newFlagSet(
	name, envPrefix string,
	cfg *Config,
	short, long *string,
	out io.Writer,
) (*flag.FlagSet, func() error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	usageOut := errwriter.New(out)
	usageCalled := false
	flags.Usage = func() {
		usageCalled = true
		usageOut.Record(fmt.Fprintf(usageOut, "Usage: %s [flags] [-- Godot arguments]\n\nFlags:\n", name))
		flags.SetOutput(usageOut)
		flags.PrintDefaults()
		flags.SetOutput(io.Discard)
	}
	usageErr := func() error {
		if usageCalled && usageOut.Err() != nil {
			return fmt.Errorf("usage: %w", usageOut.Err())
		}
		return nil
	}
	flags.BoolVar(&cfg.Version, "version", false, "print the version and exit")
	flags.StringVar(&cfg.Project, "project", "", "Godot project (env "+envPrefix+"_PROJECT takes precedence)")
	flags.TextVar(&cfg.LogLevel, "log-level", slog.LevelInfo,
		"log level: debug, info, warn, error (env "+envPrefix+"_LOG_LEVEL takes precedence)")
	flags.StringVar(&cfg.LogFile, "log-file", "", "log file, appended; empty means stderr (env "+envPrefix+"_LOG_FILE takes precedence)")
	flags.StringVar(&cfg.CPUProfile, "cpuprofile", "",
		"write a CPU profile from start to exit to file (env "+envPrefix+"_CPU_PROFILE takes precedence)")
	flags.StringVar(&cfg.MemProfile, "memprofile", "",
		"write a memory profile on exit to file (env "+envPrefix+"_MEM_PROFILE takes precedence)")
	flags.StringVar(short, "c", "", "JSON config file, below flags and env (env CONFIG and "+envPrefix+"_CONFIG take precedence)")
	flags.StringVar(long, "config", "", "same as -c")
	return flags, usageErr
}

// parseFlags parses args into flags. A help write error takes precedence over others, flag.ErrHelp is returned as is,
// and other parse errors are wrapped in ErrUsage.
func parseFlags(flags *flag.FlagSet, args []string, usageErr func() error) error {
	if err := flags.Parse(args); err != nil {
		if uerr := usageErr(); uerr != nil {
			return uerr
		}
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}
	return usageErr()
}

// applyFile fills cfg from the config file path, skipping fields whose flags are in set and empty values;
// godot_args apply only if the command line has no Godot arguments after --.
// It returns an error if the file cannot be read or log_level is invalid.
func (cfg *Config) applyFile(path string, set map[string]bool) error {
	fc, err := readFileConfig(path)
	if err != nil {
		return err
	}
	for _, s := range []struct {
		flag string
		dst  *string
		from string
	}{
		{"project", &cfg.Project, fc.Project},
		{"log-file", &cfg.LogFile, fc.LogFile},
		{"cpuprofile", &cfg.CPUProfile, fc.CPUProfile},
		{"memprofile", &cfg.MemProfile, fc.MemProfile},
	} {
		if !set[s.flag] && s.from != "" {
			*s.dst = s.from
		}
	}
	if len(cfg.Godot) == 0 && len(fc.Godot) > 0 {
		cfg.Godot = fc.Godot
	}
	if !set["log-level"] && fc.LogLevel != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(fc.LogLevel)); err != nil {
			return fmt.Errorf("config file %s: log_level: %w", path, err)
		}
	}
	return nil
}

// applyEnv overrides cfg fields with non-empty <envPrefix>_* variables.
// It returns an error if <envPrefix>_LOG_LEVEL cannot be parsed.
func (cfg *Config) applyEnv(envPrefix string, lookupEnv func(string) (string, bool)) error {
	for _, s := range []struct {
		env string
		dst *string
	}{
		{"_PROJECT", &cfg.Project},
		{"_LOG_FILE", &cfg.LogFile},
		{"_CPU_PROFILE", &cfg.CPUProfile},
		{"_MEM_PROFILE", &cfg.MemProfile},
	} {
		if v, ok := lookupEnv(envPrefix + s.env); ok && v != "" {
			*s.dst = v
		}
	}
	if v, ok := lookupEnv(envPrefix + "_LOG_LEVEL"); ok && v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return fmt.Errorf("%s_LOG_LEVEL: %w", envPrefix, err)
		}
	}
	return nil
}

// readFileConfig reads the JSON config file path, decoding straight from the file without reading it into memory first.
// Unknown fields and data after the JSON object are errors.
func readFileConfig(path string) (fc fileConfig, err error) {
	f, err := os.Open(path)
	if err != nil {
		return fc, fmt.Errorf("config file: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fc); err != nil {
		return fc, fmt.Errorf("config file %s: %w", path, err)
	}
	switch _, err := dec.Token(); {
	case errors.Is(err, io.EOF):
		return fc, nil
	case err != nil:
		return fc, fmt.Errorf("config file %s: after JSON: %w", path, err)
	default:
		return fc, fmt.Errorf("config file %s: unexpected data after JSON", path)
	}
}
