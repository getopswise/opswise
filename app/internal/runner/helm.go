package runner

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// RunHelm executes helm upgrade --install for a given release with a values file and overrides.
func RunHelm(releaseName, chart, namespace, valuesFile string, setValues map[string]string) (io.Reader, <-chan Result, error) {
	args := []string{
		"upgrade", "--install", releaseName, chart,
		"--namespace", namespace,
		"--create-namespace",
	}
	if valuesFile != "" {
		args = append(args, "-f", valuesFile)
	}
	for k, v := range setValues {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}

	cmd := exec.Command("helm", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	pr, pw := io.Pipe()
	done := make(chan Result, 1)

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start helm: %w", err)
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

// HelmValuesPath returns the filesystem path to a product's helm values.yaml.
func HelmValuesPath(deployDir, productName string) string {
	return fmt.Sprintf("%s/products/%s/helm/values.yaml", deployDir, productName)
}
