package generator

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/helm/helm/log"
)

const GeneratorKeyword = "helm:generate "

// Walk walks a chart directory and executes generators as it finds them.
//
// Returns the number of generators executed.
func Walk(dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {

		// dive-bomb if we hit an error.
		if err != nil {
			return err
		}

		// Skip directory entries. If the directory prefix is . or _, skip the
		// contents of the directory as well.
		if fi.IsDir() {
			return skip(path)
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		line, err := readGenerator(f)
		if err != nil {
			return err
		}
		if line == "" {
			return nil
		}
		// Run the generator.
		os.Setenv("HELM_GENERATE_COMMAND", line)
		os.Setenv("HELM_GENERATE_FILE", path)
		os.Setenv("HELM_GENERATE_DIR", dir)
		line = os.ExpandEnv(line)
		os.Setenv("HELM_GENERATE_COMMAND_EXPANDED", line)
		log.Debug("File: %s, Command: %s", path, line)
		count++
		err = execute(line)
		if err != nil {
			return fmt.Errorf("failed to execute %s (%s): %s", line, path, err)
		}
		return nil
	})

	return count, err
}

func execute(command string) error {
	args := strings.Fields(command)
	if len(args) == 0 {
		return errors.New("empty command")
	}
	name := args[0]
	args = args[1:]

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// skip indicates whether the directory's contents should be skipped.
//
// error is nil unless the directory passes the skip test, in which acse it is
// filepath.SkipDir
func skip(path string) error {
	base := filepath.Base(path)
	if base[0] == '.' || base[0] == '_' {
		return filepath.SkipDir
	}
	return nil
}

// Read the generator from a file.
//
// An error indicates that something went wrong.
//
// An empty string indicates that there was no generator.
//
// A string is to be treated as the value of the generator, without the
// `helm:generate` prefix.
func readGenerator(file *os.File) (string, error) {

	f := bufio.NewReader(file)

	// Look for leading `//`, `#`, or `/*`
	var b []byte
	var err error
	if b, err = f.Peek(3); err != nil {
		return "", nil
	}

	offset := 0
	suffix := ""
	if b[0] == '#' {
		offset++
		if b[1] == ' ' {
			offset++
		}
	} else if b[0] == '/' && (b[1] == '/' || b[1] == '*') {
		offset += 2
		if b[2] == ' ' {
			offset++
		}
		if b[1] == '*' {
			suffix = "*/"
		}
	} else {
		return "", nil
	}

	if _, err := f.Discard(offset); err != nil {
		return "", err
	}

	// If we get here, we have a comment header. Next, check if it's a helm:generate header.
	if b, err = f.Peek(len(GeneratorKeyword)); err != nil {
		return "", nil
	}

	slug := string(b)
	if slug != GeneratorKeyword {
		return "", nil
	}
	if _, err := f.Discard(len(GeneratorKeyword)); err != nil {
		return "", err
	}

	// At this point, we know that we have a helm:generate header. Read to EOL.
	line, err := f.ReadString('\n')
	if err != nil {
		return "", err
	}

	line = strings.TrimSpace(line)
	if len(suffix) > 0 {
		line = strings.TrimSpace(strings.TrimSuffix(line, suffix))
	}
	return line, err
}
