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
	Name     string
	Hostname string
	User     string
	Port     string
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
	authMethods, err := authMethods()
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

func authMethods() ([]ssh.AuthMethod, error) {
	// If an SSH agent is available, use it exclusively.
	// Mixing agent + key files can exceed MaxAuthTries on servers with low
	// limits (common default is 6); agents like 1Password already select the
	// right key per host, so adding file keys on top only adds failed attempts.
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			return []ssh.AuthMethod{
				ssh.PublicKeysCallback(agent.NewClient(conn).Signers),
			}, nil
		}
	}

	// No agent: fall back to well-known key files.
	home, _ := os.UserHomeDir()
	var signers []ssh.Signer
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		path := filepath.Join(home, ".ssh", name)
		key, err := os.ReadFile(path)
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
