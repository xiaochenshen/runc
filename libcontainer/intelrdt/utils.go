// +build linux

package intelrdt

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	IntelRdtTasks = "tasks"
)

var (
	ErrNotValidFormat     = errors.New("line is not a valid key value format")
	ErrIntelRdtNotEnabled = errors.New("intelrdt: config provided but Intel RDT not supported")
)

func parseUint(s string, base, bitSize int) (uint64, error) {
	value, err := strconv.ParseUint(s, base, bitSize)
	if err != nil {
		intValue, intErr := strconv.ParseInt(s, base, bitSize)
		// 1. Handle negative values greater than MinInt64 (and)
		// 2. Handle negative values lesser than MinInt64
		if intErr == nil && intValue < 0 {
			return 0, nil
		} else if intErr != nil && intErr.(*strconv.NumError).Err == strconv.ErrRange && intValue < 0 {
			return 0, nil
		}

		return value, err
	}

	return value, nil
}

// Parses a param and returns as name, value
func getIntelRdtParamKeyValue(t string) (string, uint64, error) {
	parts := strings.Fields(t)
	switch len(parts) {
	case 2:
		value, err := parseUint(parts[1], 10, 64)
		if err != nil {
			return "", 0, fmt.Errorf("unable to convert param value (%q) to uint64: %v", parts[1], err)
		}

		return parts[0], value, nil
	default:
		return "", 0, ErrNotValidFormat
	}
}

// Gets a single uint64 value from the specified file.
func getIntelRdtParamUint(path, file string) (uint64, error) {
	fileName := filepath.Join(path, file)
	contents, err := ioutil.ReadFile(fileName)
	if err != nil {
		return 0, err
	}

	res, err := parseUint(strings.TrimSpace(string(contents)), 10, 64)
	if err != nil {
		return res, fmt.Errorf("unable to parse %q as a uint from file %q", string(contents), fileName)
	}
	return res, nil
}

// Gets a string value from the specified file
func getIntelRdtParamString(path, file string) (string, error) {
	contents, err := ioutil.ReadFile(filepath.Join(path, file))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(contents)), nil
}

func writeFile(dir, file, data string) error {
	if dir == "" {
		return fmt.Errorf("no such directory for %s", file)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, file), []byte(data), 0700); err != nil {
		return fmt.Errorf("failed to write %v to %v: %v", data, file, err)
	}
	return nil
}

func readTasksFile(dir string) ([]int, error) {
	f, err := os.Open(filepath.Join(dir, IntelRdtTasks))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		s   = bufio.NewScanner(f)
		out = []int{}
	)

	for s.Scan() {
		if t := s.Text(); t != "" {
			pid, err := strconv.Atoi(t)
			if err != nil {
				return nil, err
			}
			out = append(out, pid)
		}
	}
	return out, nil
}

// Return the mount point path of Intel RDT "resource control" filesysem
func findIntelRdtMountpointDir() (string, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		text := s.Text()
		fields := strings.Split(text, " ")
		// Safe as mountinfo encodes mountpoints with spaces as \040.
		index := strings.Index(text, " - ")
		postSeparatorFields := strings.Fields(text[index+3:])
		numPostFields := len(postSeparatorFields)

		// This is an error as we can't detect if the mount is for Intel RDT
		if numPostFields == 0 {
			return "", fmt.Errorf("Found no fields post '-' in %q", text)
		}

		if postSeparatorFields[0] == "rscctrl" {
			// Check that the mount is properly formated.
			if numPostFields < 3 {
				return "", fmt.Errorf("Error found less than 3 fields post '-' in %q", text)
			}

			return fields[4], nil
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}

	return "", NewNotFoundError("intelrdt")
}

func parseCpuInfoFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if err := s.Err(); err != nil {
			return false, err
		}

		text := s.Text()
		flags := strings.Split(text, " ")

		for _, flag := range flags {
			if flag == "rdt" {
				return true, nil
			}
		}
	}
	return false, nil
}

// WriteIntelRdtTasks writes the specified pid into the tasks file
func WriteIntelRdtTasks(dir string, pid int) error {
	if dir == "" {
		return fmt.Errorf("no such directory for %s", IntelRdtTasks)
	}

	// Dont attach any pid if -1 is specified as a pid
	if pid != -1 {
		if err := ioutil.WriteFile(filepath.Join(dir, IntelRdtTasks), []byte(strconv.Itoa(pid)), 0700); err != nil {
			return fmt.Errorf("failed to write %v to %v: %v", pid, IntelRdtTasks, err)
		}
	}
	return nil
}
