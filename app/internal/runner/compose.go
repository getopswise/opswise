package runner

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// RunCompose executes docker compose up for a given compose file with env vars.
func RunCompose(composeFile string, envVars map[string]string) (io.Reader, <-chan Result, error) {
	args := []string{"compose", "-f", composeFile, "up", "-d"}

	cmd := exec.Command("docker", args...)

	// Pass env vars
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

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
		return nil, nil, fmt.Errorf("start docker compose: %w", err)
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

// ComposePath returns the filesystem path to a product's docker-compose.yml.
func ComposePath(deployDir, productName string) string {
	return fmt.Sprintf("%s/products/%s/compose/docker-compose.yml", deployDir, productName)
}
