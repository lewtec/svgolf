package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"syscall"
)

func startProfiles(dir string) (func() error, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cpu, err := os.Create(filepath.Join(dir, "cpu.pprof"))
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(cpu); err != nil {
		cpu.Close()
		return nil, err
	}
	tr, err := os.Create(filepath.Join(dir, "trace.out"))
	if err != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		return nil, err
	}
	if err := trace.Start(tr); err != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		tr.Close()
		return nil, err
	}
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)
	var (
		once    sync.Once
		stopErr error
		done    = make(chan struct{})
	)
	stop := func() error {
		once.Do(func() {
			close(done)
			trace.Stop()
			if err := tr.Close(); err != nil {
				pprof.StopCPUProfile()
				cpu.Close()
				stopErr = err
				return
			}
			pprof.StopCPUProfile()
			if err := cpu.Close(); err != nil {
				stopErr = err
				return
			}
			runtime.GC()
			for _, name := range []string{"heap", "allocs", "goroutine", "mutex", "block"} {
				if err := writeProfile(filepath.Join(dir, name+".pprof"), name); err != nil && stopErr == nil {
					stopErr = err
				}
			}
		})
		return stopErr
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
			signal.Stop(sig)
			_ = stop()
			os.Exit(130)
		case <-done:
			signal.Stop(sig)
		}
	}()
	return stop, nil
}

func writeProfile(path, name string) error {
	p := pprof.Lookup(name)
	if p == nil {
		return fmt.Errorf("profile: missing %s", name)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = p.WriteTo(f, 0)
	if c := f.Close(); err == nil {
		err = c
	}
	return err
}
