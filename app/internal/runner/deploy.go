package runner

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	gitpush "github.com/getopswise/opswise/app/internal/git"
)

// DeployService manages deployment execution and log streaming.
type DeployService struct {
	q         *dbq.Queries
	deployDir string

	mu         sync.RWMutex
	subscribers map[int64][]chan string
}

// NewDeployService creates a new deploy service.
func NewDeployService(q *dbq.Queries, deployDir string) *DeployService {
	return &DeployService{
		q:           q,
		deployDir:   deployDir,
		subscribers: make(map[int64][]chan string),
	}
}

// Subscribe returns a channel that receives log lines for the given deployment.
func (s *DeployService) Subscribe(deploymentID int64) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan string, 64)
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
	s.mu.RLock()
	defer s.mu.RUnlock()
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

	// Clean up subscribers
	s.mu.Lock()
	for _, ch := range s.subscribers[id] {
		close(ch)
	}
	delete(s.subscribers, id)
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

	var inventory []string
	for _, h := range params.Hosts {
		entry := fmt.Sprintf("%s ansible_user=%s ansible_port=%d", h.Ip, h.SshUser, h.SshPort)
		switch {
		case h.SshKey.Valid && h.SshKey.String != "":
			entry += fmt.Sprintf(" ansible_ssh_private_key_file=%s", h.SshKey.String)
		case globalSSHKey != "":
			entry += fmt.Sprintf(" ansible_ssh_private_key_file=%s", globalSSHKey)
		}
		inventory = append(inventory, entry)
	}
	appendLog(fmt.Sprintf("Inventory: %s", strings.Join(inventory, ", ")))
	appendLog("")

	return RunPlaybook(playbook, inventory, params.Config)
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
