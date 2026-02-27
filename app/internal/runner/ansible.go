package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Result captures the output and exit status of a playbook run.
type Result struct {
	ExitCode int
	Err      error
}

// RunPlaybook executes an ansible-playbook command and streams output line by line
// through the returned io.Reader. The done channel receives the result when complete.
func RunPlaybook(playbook string, inventory []string, extraVars map[string]string) (io.Reader, <-chan Result, error) {
	args := []string{
		playbook,
		"-i", strings.Join(inventory, ",") + ",",
		"--no-color",
	}
	if len(extraVars) > 0 {
		varsJSON, _ := json.Marshal(extraVars)
		args = append(args, "--extra-vars", string(varsJSON))
	}

	cmd := exec.Command("ansible-playbook", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// Merge stdout and stderr into a single reader
	pr, pw := io.Pipe()
	done := make(chan Result, 1)

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start ansible-playbook: %w", err)
	}

	go func() {
		var wg sync.WaitGroup
		wg.Add(2)

		copyLines := func(r io.Reader) {
			defer wg.Done()
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				fmt.Fprintln(pw, scanner.Text())
			}
		}

		go copyLines(stdout)
		go copyLines(stderr)

		wg.Wait()

		err := cmd.Wait()
		pw.Close()

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}

		done <- Result{ExitCode: exitCode, Err: err}
		close(done)
	}()

	return pr, done, nil
}

// PlaybookPath returns the filesystem path to a product's install playbook.
func PlaybookPath(deployDir, productName string) string {
	return fmt.Sprintf("%s/products/%s/ansible/install.yml", deployDir, productName)
}

// UninstallPlaybookPath returns the filesystem path to a product's uninstall playbook.
func UninstallPlaybookPath(deployDir, productName string) string {
	return fmt.Sprintf("%s/products/%s/ansible/uninstall.yml", deployDir, productName)
}
