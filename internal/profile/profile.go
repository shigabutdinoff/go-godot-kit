// Package profile writes the CPU profile of a run and the heap profile on exit.
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"github.com/shigabutdinoff/go-godot-kit/internal/atomicfile"
)

// Start starts CPU profiling into the file cpu, if set. stop stops it
// and writes a memory profile to mem, if set. A relative mem is resolved against the working
// directory at call time, since Godot later changes into the project directory.
func Start(cpu, mem string) (stop func() error, err error) {
	if mem != "" {
		if mem, err = filepath.Abs(mem); err != nil {
			return nil, fmt.Errorf("memory profile: %w", err)
		}
	}
	var cpuFile *os.File
	if cpu != "" {
		f, err := os.Create(cpu)
		if err != nil {
			return nil, fmt.Errorf("CPU profile: %w", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return nil, errors.Join(fmt.Errorf("CPU profile: %w", err), f.Close())
		}
		cpuFile = f
	}
	return func() error {
		var errs []error
		if cpuFile != nil {
			pprof.StopCPUProfile()
			errs = append(errs, cpuFile.Close())
		}
		if mem != "" {
			errs = append(errs, writeHeapProfile(mem))
		}
		return errors.Join(errs...)
	}, nil
}

// writeHeapProfile writes a heap profile to path after a garbage collection, through a temporary file,
// so an interrupted write never leaves a broken profile.
func writeHeapProfile(path string) error {
	runtime.GC()
	if err := atomicfile.WriteFrom(path, 0o644, pprof.WriteHeapProfile); err != nil {
		return fmt.Errorf("memory profile: %w", err)
	}
	return nil
}
