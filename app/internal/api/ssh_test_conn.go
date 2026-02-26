package api

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
	"golang.org/x/crypto/ssh"
)

// TestSSHConnection tries to establish an SSH connection to the host.
// Fallback chain: per-host key -> per-host password -> global key -> default keys (~/.ssh/id_rsa, etc.)
func TestSSHConnection(host dbq.Host, globalSSHKey string) templates.SSHTestResult {
	addr := fmt.Sprintf("%s:%d", host.Ip, host.SshPort)
	timeout := 10 * time.Second

	// Try per-host key
	if host.SshKey.Valid && host.SshKey.String != "" {
		if result, ok := tryKeyAuth(host.SshUser, addr, host.SshKey.String, timeout); ok {
			return result
		}
	}

	// Try per-host password
	if host.SshPassword.Valid && host.SshPassword.String != "" {
		if result, ok := tryPasswordAuth(host.SshUser, addr, host.SshPassword.String, timeout); ok {
			return result
		}
	}

	// Try global key
	if globalSSHKey != "" {
		if result, ok := tryKeyAuth(host.SshUser, addr, globalSSHKey, timeout); ok {
			return result
		}
	}

	// Try default keys
	home, _ := os.UserHomeDir()
	defaultKeys := []string{"id_rsa", "id_ed25519", "id_ecdsa"}
	for _, name := range defaultKeys {
		keyPath := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(keyPath); err == nil {
			if result, ok := tryKeyAuth(host.SshUser, addr, keyPath, timeout); ok {
				return result
			}
		}
	}

	return templates.SSHTestResult{
		Success: false,
		Message: "all authentication methods failed",
	}
}

func tryKeyAuth(user, addr, keyPath string, timeout time.Duration) (templates.SSHTestResult, bool) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return templates.SSHTestResult{}, false
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return templates.SSHTestResult{}, false
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	return dialSSH(addr, config, "key:"+keyPath)
}

func tryPasswordAuth(user, addr, password string, timeout time.Duration) (templates.SSHTestResult, bool) {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	return dialSSH(addr, config, "password")
}

func dialSSH(addr string, config *ssh.ClientConfig, method string) (templates.SSHTestResult, bool) {
	conn, err := net.DialTimeout("tcp", addr, config.Timeout)
	if err != nil {
		return templates.SSHTestResult{
			Success: false,
			Message: fmt.Sprintf("tcp dial failed: %v", err),
		}, true // return true to stop trying - host unreachable
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return templates.SSHTestResult{}, false // auth failed, try next method
	}

	client := ssh.NewClient(sshConn, chans, reqs)
	client.Close()

	return templates.SSHTestResult{
		Success: true,
		Message: "auth:" + method,
	}, true
}
