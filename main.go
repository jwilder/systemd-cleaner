package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const sessionRoot = "/run/systemd/system"

var (
	verbose bool
	checkInterval time.Duration
	debug *log.Logger = log.New(ioutil.Discard, "", log.LstdFlags)
	info *log.Logger = log.New(os.Stdout, "", log.LstdFlags)
)

func main() {
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")
	flag.DurationVar(&checkInterval, "check-interval", 10 * time.Minute, "Frequency that the cleaner runs")
	flag.Parse()

	if verbose {
		debug.SetOutput(os.Stdout)
	}

	info.Printf("Starting systemd-cleaner checker-interval=%s\n", checkInterval.String())
	t := time.NewTicker(checkInterval)
	defer t.Stop()

	if err := cleanup(); err != nil {
		info.Println(err.Error())
	}

	for {
		<-t.C
		if err := cleanup(); err != nil {
			info.Println(err.Error())
		}
	}
}

func cleanup() error {
	var dirsTotal, dirsRemoved, unitsTotal, unitsStopped int

	info.Println("Starting cleanup...")
	// The leaked resources are located in the systemd runtime dir.  First get the list.  This should
	// be a few hundred, but lots of leaked resources can be 10's of thousands.
	dirs, err := ioutil.ReadDir(sessionRoot)
	if err != nil {
		return err
	}
	defer func() {
		info.Printf("Cleaned %d dirs out of %d, %d units out of %d\n", dirsRemoved, dirsTotal, unitsStopped, unitsTotal)
	}()
	// Retrieve the set of valid pod IDs based on kubelet runtime dirs
	pods := readPods()

	info.Printf("Found %d pods\n", len(pods))

	dirsTotal = len(dirs)
	info.Printf("Found %d possible leaked dirs\n", dirsTotal)
	// We first remove all the dirs that are leaked.  We do this by iterating each dir and looking the
	// leaked file mount spec contents.  The contents of this file contains the pod ID that the mount
	// was for.  We then check if that pod is still active.  If it's not, we remove the dir.
	for _, f := range dirs {
		if matched, err := regexp.MatchString("run-.*\\.scope", f.Name()); matched && err == nil {
			pod := determinePod(f.Name())
			if _, ok := pods[pod]; !ok {
				debug.Println(f.Name(), "is not in use. Removing.")
				if err := os.RemoveAll(filepath.Join(sessionRoot, f.Name())); err != nil {
					info.Println(err)
					continue
				}
				dirsRemoved++
			}
		} else if err != nil {
			info.Print(err.Error())
		}
	}
	// Now we need to remove the leaked units that are still loaded by systemd.  We do this by get a list
	// of all the units that match the run-*.scope pattern.  These correspond the the directories we scanned
	// earlier.  If the directory doesn't exist, we stop the unit via `systemctl stop` which removes them
	// the resident state because they are transient units.
	units := listUnits()

	unitsTotal = len(units)
	info.Printf("Found %d possible leaked units\n", unitsTotal)
	for _, f := range units {
		fields := strings.Fields(f)

		if len(fields) == 0 {
			continue
		}

		id := fields[0]
		if matched, err := regexp.MatchString("run-.*\\.scope", id); matched && err == nil {
			if _, err := os.Stat(filepath.Join(sessionRoot, fmt.Sprintf("%s.d", id))); err != nil && os.IsNotExist(err) {
				debug.Println("Stopping unit ", id)
				stopUnit(id)
				unitsStopped++
			}

		} else if err != nil {
			info.Println(err.Error())
		}
	}
	return nil
}

func readPods() map[string]struct{} {
	dirs, err := ioutil.ReadDir("/var/lib/kubelet/pods/")
	if err != nil {
		info.Println(err)
		return nil
	}

	pods := make(map[string]struct{}, len(dirs))
	for _, f := range dirs {
		pods[f.Name()] = struct{}{}
	}
	return pods
}

func determinePod(name string) string {
	b, err := ioutil.ReadFile(filepath.Join(sessionRoot, name, "50-Description.conf"))
	if err != nil {
		info.Println(err.Error())
		return ""
	}

	r, err := regexp.Compile("/var/lib/kubelet/pods/([^/]+)/")
	if err != nil {
		info.Println(err.Error())
		return ""
	}
	matches := r.FindSubmatch(b)
	if len(matches) == 2 {
		return string(matches[1])
	}
	return ""

}

func listUnits() []string {
	var b bytes.Buffer
	cmd := exec.Command("systemctl",  "list-units")
	cmd.Stdout = &b
	if err := cmd.Run(); err != nil {
		log.Print(err.Error())
		return nil
	}
	return strings.Split(b.String(), "\n")
}


func stopUnit(id string) {
	cmd := exec.Command("systemctl",  "stop", id)
	if err := cmd.Run(); err != nil {
		info.Println(err.Error())
	}
}