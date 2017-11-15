package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/alecthomas/kingpin"
)

func main() {
	dir := kingpin.Arg("dir", "Base directory").Default(".").ExistingDir()
	showAll := kingpin.Flag("all", "Show all checked repositories, even the clean ones").Bool()
	pull := kingpin.Flag("pull", "Pull down remote changes if behind").Bool()
	par := kingpin.Flag("par", "Parallel instances of git status to run").Default(strconv.Itoa(runtime.NumCPU())).Int()
	kingpin.Parse()

	gits := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < *par; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processGits(gits, *showAll, *pull)
		}()
	}

	err := findGits(gits, *dir)
	if err != nil {
		log.Fatal(err)
	}
	close(gits)

	wg.Wait()
}

func processGits(gits <-chan string, showAll, pull bool) {
	for dir := range gits {
		status, err := gitStatus(dir)
		if err != nil {
			fmt.Printf("?? %s: %v\n", dir, err)
			continue
		}

		if status.minus < 0 {
			gitPull(dir)
			status, err = gitStatus(dir)
			if err != nil {
				fmt.Printf("?? %s: %v\n", dir, err)
				continue
			}
		}

		if showAll || !status.clean() {
			fmt.Printf("%s %s\n", status, dir)
			continue
		}
	}
}

func findGits(gits chan<- string, dir string) error {
	curDir := ""
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if curDir != "" && strings.HasPrefix(path, curDir+string(os.PathSeparator)) {
			// in a dir where we have already detected a .git dir, skip skip skip
			return filepath.SkipDir
		}
		if filepath.Base(path) == ".git" {
			curDir = filepath.Dir(path)
			gits <- curDir
		}
		return nil
	})
}

type status struct {
	dirty       bool
	remoteError bool
	plus        int
	minus       int
}

func (s status) clean() bool {
	return s == status{}
}

func (s status) String() string {
	var ss string
	if s.dirty {
		ss += "D"
	} else {
		ss += " "
	}
	if s.remoteError {
		ss += "E"
	} else if s.plus > 0 && s.minus < 0 {
		ss += "*"
	} else if s.plus > 0 {
		ss += "+"
	} else if s.minus < 0 {
		ss += "-"
	} else {
		ss += " "
	}
	return ss
}

func gitStatus(dir string) (status, error) {
	var s status

	cmd := exec.Command("git", "remote", "update")
	cmd.Dir = dir
	if _, err := cmd.Output(); err != nil {
		s.remoteError = true
	}

	cmd = exec.Command("git", "status", "--porcelain=2", "--branch")
	cmd.Dir = dir
	bs, err := cmd.Output()
	if err != nil {
		return status{}, err
	}
	for _, line := range strings.Split(string(bs), "\n") {
		if line == "" {
			continue
		} else if strings.HasPrefix(line, "# branch.ab ") {
			fields := strings.Fields(line)
			s.plus, _ = strconv.Atoi(fields[2])
			s.minus, _ = strconv.Atoi(fields[3])
			continue
		} else if strings.HasPrefix(line, "# ") {
			continue
		}
		s.dirty = true
	}

	return s, nil
}

func gitPull(dir string) error {
	cmd := exec.Command("git", "pull", "--ff-only")
	cmd.Dir = dir
	return cmd.Run()
}
