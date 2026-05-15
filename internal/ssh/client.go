package ssh

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Host struct {
	Name         string
	Hostname     string
	User         string
	Port         string
	IdentityFile string // path to private key; .pub counterpart used for agent filtering
}

func ParseSSHConfig() ([]Host, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open ssh config: %w", err)
	}
	defer f.Close()

	var hosts []Host
	var current *Host

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "host":
			if current != nil && current.Name != "*" && !strings.ContainsAny(current.Name, "*?") {
				hosts = append(hosts, *current)
			}
			current = &Host{Name: val, Port: "22"}
		case "hostname":
			if current != nil {
				current.Hostname = val
			}
		case "user":
			if current != nil {
				current.User = val
			}
		case "port":
			if current != nil {
				current.Port = val
			}
		case "identityfile":
			if current != nil && current.IdentityFile == "" {
				current.IdentityFile = expandTilde(val)
			}
		}
	}
	if current != nil && current.Name != "*" && !strings.ContainsAny(current.Name, "*?") {
		hosts = append(hosts, *current)
	}

	for i := range hosts {
		if hosts[i].Hostname == "" {
			hosts[i].Hostname = hosts[i].Name
		}
		if hosts[i].User == "" {
			if u := os.Getenv("USER"); u != "" {
				hosts[i].User = u
			}
		}
	}

	return hosts, scanner.Err()
}

type Client struct {
	ssh  *ssh.Client
	SFTP *sftp.Client
}

func Connect(host Host) (*Client, error) {
	authMethods, err := buildAuthMethods(host)
	if err != nil {
		return nil, err
	}

	home, _ := os.UserHomeDir()
	knownHostsFile := filepath.Join(home, ".ssh", "known_hosts")
	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	if _, err := os.Stat(knownHostsFile); err == nil {
		cb, err := knownhosts.New(knownHostsFile)
		if err == nil {
			hostKeyCallback = cb
		}
	}

	cfg := &ssh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
	}

	addr := net.JoinHostPort(host.Hostname, host.Port)
	sc, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	sftpClient, err := sftp.NewClient(sc)
	if err != nil {
		sc.Close()
		return nil, fmt.Errorf("sftp client: %w", err)
	}

	return &Client{ssh: sc, SFTP: sftpClient}, nil
}

func (c *Client) Close() {
	c.SFTP.Close()
	c.ssh.Close()
}

// buildAuthMethods returns the auth methods to use for the given host.
// When SSH_AUTH_SOCK is set, only the agent is used. If the host specifies an
// IdentityFile, only the agent signer whose public key matches that file is
// offered — this prevents MaxAuthTries failures on servers with low limits
// when the agent (e.g. 1Password) holds many keys.
func buildAuthMethods(host Host) ([]ssh.AuthMethod, error) {
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			agentClient := agent.NewClient(conn)
			if host.IdentityFile != "" {
				// Try to read the matching public key to filter agent signers.
				if method, ok := agentMethodForIdentity(agentClient, host.IdentityFile); ok {
					return []ssh.AuthMethod{method}, nil
				}
			}
			// No IdentityFile or pub key not readable — offer all agent keys.
			return []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)}, nil
		}
	}

	// No agent: fall back to well-known key files.
	home, _ := os.UserHomeDir()
	candidates := []string{"id_ed25519", "id_rsa", "id_ecdsa"}
	if host.IdentityFile != "" {
		candidates = []string{host.IdentityFile}
	}
	var signers []ssh.Signer
	for _, p := range candidates {
		if !filepath.IsAbs(p) {
			p = filepath.Join(home, ".ssh", p)
		}
		key, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			continue
		}
		signers = append(signers, signer)
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no SSH auth methods available (no agent and no key files found)")
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil
}

// agentMethodForIdentity returns a PublicKeys auth method containing only the
// agent signer whose public key matches identityFile. Accepts both the private
// key path (reads the adjacent .pub) and the .pub path directly.
func agentMethodForIdentity(agentClient agent.ExtendedAgent, identityFile string) (ssh.AuthMethod, bool) {
	pubKeyFile := identityFile
	if !strings.HasSuffix(identityFile, ".pub") {
		pubKeyFile = identityFile + ".pub"
	}
	pubKeyBytes, err := os.ReadFile(pubKeyFile)
	if err != nil {
		return nil, false
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubKeyBytes)
	if err != nil {
		return nil, false
	}
	wantFP := ssh.FingerprintSHA256(pubKey)

	signers, err := agentClient.Signers()
	if err != nil {
		return nil, false
	}
	for _, s := range signers {
		if ssh.FingerprintSHA256(s.PublicKey()) == wantFP {
			return ssh.PublicKeys(s), true
		}
	}
	return nil, false
}

func expandTilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func (c *Client) ListDirs(path string) ([]string, error) {
	entries, err := c.SFTP.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("list dir %s: %w", path, err)
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

func (c *Client) UploadFile(localPath, remotePath string) error {
	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local %s: %w", localPath, err)
	}
	defer src.Close()

	if err := c.SFTP.MkdirAll(filepath.Dir(remotePath)); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", filepath.Dir(remotePath), err)
	}

	dst, err := c.SFTP.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	defer dst.Close()

	if _, err := dst.ReadFrom(src); err != nil {
		return fmt.Errorf("upload %s: %w", localPath, err)
	}
	return nil
}

// UploadBytes writes content to remotePath, creating parent directories.
func (c *Client) UploadBytes(remotePath string, content []byte) error {
	if err := c.SFTP.MkdirAll(filepath.Dir(remotePath)); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", filepath.Dir(remotePath), err)
	}

	dst, err := c.SFTP.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	defer dst.Close()

	if _, err := dst.Write(content); err != nil {
		return fmt.Errorf("write %s: %w", remotePath, err)
	}
	return nil
}

// Remove deletes remotePath. Missing files are not an error.
func (c *Client) Remove(remotePath string) error {
	if err := c.SFTP.Remove(remotePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove %s: %w", remotePath, err)
	}
	return nil
}
