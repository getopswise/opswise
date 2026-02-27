package runner

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/getopswise/opswise/app/internal/crypto"
	"github.com/getopswise/opswise/app/internal/db/dbq"
	gitpush "github.com/getopswise/opswise/app/internal/git"
)

// DeployService manages deployment execution and log streaming.
type DeployService struct {
	q         *dbq.Queries
	deployDir string
	masterKey []byte

	mu          sync.RWMutex
	subscribers map[int64][]chan string
	liveLogs    map[int64][]string // accumulated log lines for running deployments
}

// NewDeployService creates a new deploy service.
func NewDeployService(q *dbq.Queries, deployDir string, masterKey []byte) *DeployService {
	return &DeployService{
		q:           q,
		deployDir:   deployDir,
		masterKey:   masterKey,
		subscribers: make(map[int64][]chan string),
		liveLogs:    make(map[int64][]string),
	}
}

// Subscribe returns a channel that receives log lines for the given deployment.
// It also replays any accumulated log lines so the subscriber catches up.
func (s *DeployService) Subscribe(deploymentID int64) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan string, 256)
	// Replay accumulated log lines
	for _, line := range s.liveLogs[deploymentID] {
		ch <- line
	}
	s.subscribers[deploymentID] = append(s.subscribers[deploymentID], ch)
	return ch
}

// Unsubscribe removes a subscriber channel.
func (s *DeployService) Unsubscribe(deploymentID int64, ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := s.subscribers[deploymentID]
	for i, sub := range subs {
		if sub == ch {
			s.subscribers[deploymentID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (s *DeployService) broadcast(deploymentID int64, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if line != "" {
		s.liveLogs[deploymentID] = append(s.liveLogs[deploymentID], line)
	}
	for _, ch := range s.subscribers[deploymentID] {
		select {
		case ch <- line:
		default:
			// drop if subscriber is slow
		}
	}
}

// StartDeployment creates a DB record and launches the deployment in a goroutine.
func (s *DeployService) StartDeployment(ctx context.Context, params DeployParams) (int64, error) {
	hostIDsJSON, err := json.Marshal(params.HostIDs)
	if err != nil {
		return 0, fmt.Errorf("marshal host ids: %w", err)
	}
	configJSON, err := json.Marshal(params.Config)
	if err != nil {
		return 0, fmt.Errorf("marshal config: %w", err)
	}

	dep, err := s.q.CreateDeployment(ctx, dbq.CreateDeploymentParams{
		Name:       params.Name,
		Type:       params.Type,
		TargetName: params.TargetName,
		Mode:       params.Mode,
		HostIds:    string(hostIDsJSON),
		Config:     sql.NullString{String: string(configJSON), Valid: true},
		Status:     "pending",
	})
	if err != nil {
		return 0, fmt.Errorf("create deployment: %w", err)
	}

	go s.runDeployment(dep.ID, params)

	return dep.ID, nil
}

// DeployParams holds the parameters for a deployment.
type DeployParams struct {
	Name       string
	Type       string // "product" or "stack"
	TargetName string
	Mode       string // "ansible", "compose", "helm"
	HostIDs    []int64
	Config     map[string]string
	Hosts      []dbq.Host // resolved host objects
}

func (s *DeployService) runDeployment(id int64, params DeployParams) {
	ctx := context.Background()

	// Mark as running
	s.q.UpdateDeploymentStatus(ctx, dbq.UpdateDeploymentStatusParams{
		ID: id, Status: "running",
	})

	var logBuf strings.Builder
	appendLog := func(line string) {
		logBuf.WriteString(line)
		logBuf.WriteString("\n")
		s.broadcast(id, line)
	}

	appendLog(fmt.Sprintf("=== Deployment #%d started ===", id))
	appendLog(fmt.Sprintf("Target: %s (%s)", params.TargetName, params.Type))
	appendLog(fmt.Sprintf("Mode: %s", params.Mode))
	appendLog("")

	var reader io.Reader
	var done <-chan Result
	var err error

	switch params.Mode {
	case "ansible":
		reader, done, err = s.runAnsible(params, appendLog)
	case "compose":
		reader, done, err = s.runComposeMode(params, appendLog)
	case "helm":
		reader, done, err = s.runHelmMode(params, appendLog)
	default:
		appendLog(fmt.Sprintf("ERROR: unknown mode %q", params.Mode))
		s.finishDeployment(ctx, id, "failed", logBuf.String())
		return
	}

	if err != nil {
		appendLog(fmt.Sprintf("ERROR: %v", err))
		s.finishDeployment(ctx, id, "failed", logBuf.String())
		return
	}

	// Stream output
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		appendLog(scanner.Text())
	}

	// Wait for completion
	result := <-done

	appendLog("")
	if result.ExitCode == 0 {
		appendLog("=== Deployment completed successfully ===")
		// Resolve product metadata (GUI URL, credentials)
		meta := LoadProductMeta(s.deployDir, params.TargetName)
		if len(params.Hosts) > 0 {
			hostIP := params.Hosts[0].Ip
			if guiURL := ResolveTemplate(meta.GUIURL, hostIP, params.Config); guiURL != "" {
				s.q.UpdateDeploymentGUIURL(ctx, dbq.UpdateDeploymentGUIURLParams{
					ID:     id,
					GuiUrl: sql.NullString{String: guiURL, Valid: true},
				})
			}
			loginUser := ResolveTemplate(meta.LoginUser, hostIP, params.Config)
			loginPass := ResolveTemplate(meta.LoginPassword, hostIP, params.Config)
			if loginUser != "" || loginPass != "" {
				s.q.UpdateDeploymentCredentials(ctx, dbq.UpdateDeploymentCredentialsParams{
					ID:            id,
					LoginUser:     sql.NullString{String: loginUser, Valid: loginUser != ""},
					LoginPassword: sql.NullString{String: loginPass, Valid: loginPass != ""},
				})
			}
		}
		s.finishDeployment(ctx, id, "success", logBuf.String())
		s.tryGitPush(ctx, id, params, appendLog)
	} else {
		appendLog(fmt.Sprintf("=== Deployment failed (exit code %d) ===", result.ExitCode))
		s.finishDeployment(ctx, id, "failed", logBuf.String())
	}
}

func (s *DeployService) finishDeployment(ctx context.Context, id int64, status, logOutput string) {
	if err := s.q.UpdateDeploymentStatus(ctx, dbq.UpdateDeploymentStatusParams{
		ID: id, Status: status,
	}); err != nil {
		log.Printf("update deployment %d status: %v", id, err)
	}
	if err := s.q.UpdateDeploymentLog(ctx, dbq.UpdateDeploymentLogParams{
		ID:  id,
		Log: sql.NullString{String: logOutput, Valid: true},
	}); err != nil {
		log.Printf("update deployment %d log: %v", id, err)
	}

	// Broadcast completion sentinel
	s.broadcast(id, "")

	// Clean up subscribers and live log buffer
	s.mu.Lock()
	for _, ch := range s.subscribers[id] {
		close(ch)
	}
	delete(s.subscribers, id)
	delete(s.liveLogs, id)
	s.mu.Unlock()
}

func (s *DeployService) tryGitPush(ctx context.Context, id int64, params DeployParams, appendLog func(string)) {
	// Check if git is enabled
	enabled, err := s.q.GetSetting(ctx, "git_enabled")
	if err != nil || enabled != "true" {
		return
	}

	gitURL, _ := s.q.GetSetting(ctx, "git_url")
	if gitURL == "" {
		return
	}

	gitBranch, _ := s.q.GetSetting(ctx, "git_branch")
	gitToken, _ := s.q.GetSetting(ctx, "git_token")

	appendLog("")
	appendLog("=== Pushing to Git repository ===")
	appendLog(fmt.Sprintf("Repository: %s", gitURL))
	appendLog(fmt.Sprintf("Branch: %s", gitBranch))

	cfg := gitpush.PushConfig{
		URL:    gitURL,
		Branch: gitBranch,
		Token:  gitToken,
	}

	if err := gitpush.PushDeployment(cfg, s.deployDir, id, params.TargetName, params.Mode); err != nil {
		appendLog(fmt.Sprintf("Git push failed: %v", err))
		log.Printf("git push deployment %d: %v", id, err)
		return
	}

	appendLog("Git push successful.")
	if err := s.q.SetDeploymentGitPushed(ctx, id); err != nil {
		log.Printf("set git pushed deployment %d: %v", id, err)
	}
}

func (s *DeployService) runAnsible(params DeployParams, appendLog func(string)) (io.Reader, <-chan Result, error) {
	playbook := PlaybookPath(s.deployDir, params.TargetName)
	appendLog(fmt.Sprintf("Playbook: %s", playbook))

	// Load global SSH key as fallback
	globalSSHKey, _ := s.q.GetSetting(context.Background(), "ssh_key_path")

	// Track temp files for cleanup
	var tmpFiles []string
	cleanup := func() {
		for _, f := range tmpFiles {
			os.Remove(f)
		}
	}

	var inventory []string
	for _, h := range params.Hosts {
		entry := fmt.Sprintf("%s ansible_user=%s ansible_port=%d", h.Ip, h.SshUser, h.SshPort)
		if h.SshKey.Valid && h.SshKey.String != "" {
			// Decrypt key content and write to temp file
			keyData, err := crypto.Decrypt(h.SshKey.String, s.masterKey)
			if err != nil {
				log.Printf("failed to decrypt SSH key for host %s: %v", h.Name, err)
			} else {
				tmpFile, err := os.CreateTemp("", "opswise-ssh-*")
				if err != nil {
					log.Printf("failed to create temp key file for host %s: %v", h.Name, err)
				} else {
					if _, err := tmpFile.Write(keyData); err != nil {
						tmpFile.Close()
						os.Remove(tmpFile.Name())
						log.Printf("failed to write temp key file for host %s: %v", h.Name, err)
					} else {
						tmpFile.Close()
						os.Chmod(tmpFile.Name(), 0600)
						tmpFiles = append(tmpFiles, tmpFile.Name())
						entry += fmt.Sprintf(" ansible_ssh_private_key_file=%s", tmpFile.Name())
						appendLog(fmt.Sprintf("Host %s: using encrypted SSH key", h.Name))
					}
				}
			}
		} else if globalSSHKey != "" {
			entry += fmt.Sprintf(" ansible_ssh_private_key_file=%s", globalSSHKey)
		}
		inventory = append(inventory, entry)
	}
	appendLog(fmt.Sprintf("Inventory: %s", strings.Join(inventory, ", ")))
	appendLog("")

	reader, done, err := RunPlaybook(playbook, inventory, params.Config)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	// Wrap the done channel to clean up temp files after completion
	wrappedDone := make(chan Result, 1)
	go func() {
		result := <-done
		cleanup()
		wrappedDone <- result
	}()

	return reader, wrappedDone, nil
}

func (s *DeployService) runComposeMode(params DeployParams, appendLog func(string)) (io.Reader, <-chan Result, error) {
	composeFile := ComposePath(s.deployDir, params.TargetName)
	appendLog(fmt.Sprintf("Compose file: %s", composeFile))
	appendLog("")

	return RunCompose(composeFile, params.Config)
}

func (s *DeployService) runHelmMode(params DeployParams, appendLog func(string)) (io.Reader, <-chan Result, error) {
	valuesFile := HelmValuesPath(s.deployDir, params.TargetName)
	releaseName := params.TargetName
	chart := params.TargetName // chart name, could be overridden
	namespace := "default"

	if ns, ok := params.Config["namespace"]; ok {
		namespace = ns
		delete(params.Config, "namespace")
	}
	if c, ok := params.Config["chart"]; ok {
		chart = c
		delete(params.Config, "chart")
	}

	appendLog(fmt.Sprintf("Release: %s", releaseName))
	appendLog(fmt.Sprintf("Chart: %s", chart))
	appendLog(fmt.Sprintf("Namespace: %s", namespace))
	appendLog(fmt.Sprintf("Values: %s", valuesFile))
	appendLog("")

	return RunHelm(releaseName, chart, namespace, valuesFile, params.Config)
}
