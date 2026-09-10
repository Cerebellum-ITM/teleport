package ssh

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ErrNoAuthMethods is returned by Connect when no SSH agent is running and no
// key files are found. Callers may catch it to prompt for a password.
var ErrNoAuthMethods = errors.New("no SSH auth methods available")

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

	sftpClient, err := sftp.NewClient(sc,
		sftp.MaxPacket(32768), // 32 KB: max payload accepted by OpenSSH sftp-server (SFTP_MAX_MSG_LENGTH)
		sftp.UseConcurrentWrites(true),
	)
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
		return nil, ErrNoAuthMethods
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil
}

// ConnectWithPassword connects to host using password authentication only.
func ConnectWithPassword(host Host, password string) (*Client, error) {
	home, _ := os.UserHomeDir()
	knownHostsFile := filepath.Join(home, ".ssh", "known_hosts")
	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	if _, err := os.Stat(knownHostsFile); err == nil {
		if cb, err := knownhosts.New(knownHostsFile); err == nil {
			hostKeyCallback = cb
		}
	}

	cfg := &ssh.ClientConfig{
		User:            host.User,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: hostKeyCallback,
	}

	addr := net.JoinHostPort(host.Hostname, host.Port)
	sc, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	sftpClient, err := sftp.NewClient(sc,
		sftp.MaxPacket(32768), // MaxPacketChecked rejects >32 KB; this is the safe universal payload
		sftp.UseConcurrentWrites(true),
	)
	if err != nil {
		sc.Close()
		return nil, fmt.Errorf("sftp client: %w", err)
	}

	return &Client{ssh: sc, SFTP: sftpClient}, nil
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
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

// Mkdir creates dir on the remote. It is idempotent: an already-existing
// directory is not an error. Parent directories must already exist.
func (c *Client) Mkdir(dir string) error {
	if err := c.SFTP.Mkdir(dir); err != nil {
		// Tolerate "already exists": re-stat and accept if it is a dir.
		if info, statErr := c.SFTP.Stat(dir); statErr == nil && info.IsDir() {
			return nil
		}
		return fmt.Errorf("mkdir remote %s: %w", dir, err)
	}
	return nil
}

// MkdirAll creates dir and any missing parent directories on the remote.
func (c *Client) MkdirAll(dir string) error {
	if err := c.SFTP.MkdirAll(dir); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", dir, err)
	}
	return nil
}

// Chmod sets the mode of remotePath. Needed because SFTP.Create keeps the mode
// of an already-existing remote file, so an overwrite would otherwise inherit a
// stale mode.
func (c *Client) Chmod(remotePath string, mode os.FileMode) error {
	if err := c.SFTP.Chmod(remotePath, mode); err != nil {
		return fmt.Errorf("chmod remote %s: %w", remotePath, err)
	}
	return nil
}

func (c *Client) UploadFile(localPath, remotePath string) error {
	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local %s: %w", localPath, err)
	}
	defer src.Close()

	stat, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", localPath, err)
	}

	if err := c.SFTP.MkdirAll(filepath.Dir(remotePath)); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", filepath.Dir(remotePath), err)
	}

	dst, err := c.SFTP.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	defer dst.Close() // safety net for error paths; the success path closes explicitly below

	if _, err := dst.ReadFrom(src); err != nil {
		return fmt.Errorf("upload %s: %w", localPath, err)
	}
	// Some SFTP servers ack buffered writes but only report failure on close,
	// so the close error must be checked or a discarded upload looks successful.
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close remote %s: %w", remotePath, err)
	}
	return c.verifyUpload(remotePath, stat.Size())
}

// UploadFileProgress is like UploadFile but calls progress(written, total)
// after each chunk so callers can display transfer progress.
func (c *Client) UploadFileProgress(localPath, remotePath string, progress func(written, total int64)) error {
	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local %s: %w", localPath, err)
	}
	defer src.Close()

	stat, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", localPath, err)
	}
	total := stat.Size()

	if err := c.SFTP.MkdirAll(filepath.Dir(remotePath)); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", filepath.Dir(remotePath), err)
	}

	dst, err := c.SFTP.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	defer dst.Close() // safety net for error paths; the success path closes explicitly below

	pr := &progressReader{r: src, total: total, fn: progress}
	if _, err := dst.ReadFrom(pr); err != nil {
		return fmt.Errorf("upload %s: %w", localPath, err)
	}
	// Some SFTP servers ack buffered writes but only report failure on close,
	// so the close error must be checked or a discarded upload looks successful.
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close remote %s: %w", remotePath, err)
	}
	return c.verifyUpload(remotePath, total)
}

// progressReader wraps a reader and calls fn after each Read so callers can
// track upload progress. Exposing Size() lets sftp.File.ReadFrom take the
// concurrent-write path (pipelined writes) instead of the slow sequential one.
type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	fn    func(written, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.fn != nil && n > 0 {
		p.fn(p.read, p.total)
	}
	return n, err
}

// Size reports the total upload size so sftp.File.ReadFrom selects the
// concurrent path; it does not satisfy io.Seeker, only the size hint.
func (p *progressReader) Size() int64 { return p.total }

// UploadBytes writes content to remotePath, creating parent directories.
func (c *Client) UploadBytes(remotePath string, content []byte) error {
	if err := c.SFTP.MkdirAll(filepath.Dir(remotePath)); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", filepath.Dir(remotePath), err)
	}

	dst, err := c.SFTP.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	defer dst.Close() // safety net for error paths; the success path closes explicitly below

	if _, err := dst.Write(content); err != nil {
		return fmt.Errorf("write %s: %w", remotePath, err)
	}
	// Some SFTP servers ack buffered writes but only report failure on close,
	// so the close error must be checked or a discarded upload looks successful.
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close remote %s: %w", remotePath, err)
	}
	return c.verifyUpload(remotePath, int64(len(content)))
}

// verifyUpload confirms the remote file size matches want. It guards against
// servers that ack writes but discard the payload, which would otherwise leave
// a 0-byte file reported as a successful upload.
func (c *Client) verifyUpload(remotePath string, want int64) error {
	st, err := c.SFTP.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("verify remote %s: %w", remotePath, err)
	}
	if st.Size() != want {
		return fmt.Errorf("verify remote %s: size mismatch (local %d, remote %d)", remotePath, want, st.Size())
	}
	return nil
}

// RemoteSHA256 streams remotePath and returns its lowercase hex SHA256.
// When the file does not exist, returns ("", os.ErrNotExist).
func (c *Client) RemoteSHA256(remotePath string) (string, error) {
	f, err := c.SFTP.Open(remotePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", os.ErrNotExist
		}
		return "", fmt.Errorf("open remote %s: %w", remotePath, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read remote %s: %w", remotePath, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RemoteStat is the metadata push needs to decide whether a file may have
// changed: its size, and its modification time as a Unix timestamp.
type RemoteStat struct {
	Size    int64
	ModTime int64
}

// RemoteStatMany returns the size and mtime of every path that exists remotely,
// in one command per batch. It is the cheap first question — metadata, not
// content — that lets a caller avoid hashing files whose size already proves they
// differ. Paths that do not exist are absent from the result.
func (c *Client) RemoteStatMany(paths []string, onProgress func(done, total int)) (map[string]RemoteStat, error) {
	stats := make(map[string]RemoteStat, len(paths))
	batch := remoteBatchSize(len(paths))
	for start := 0; start < len(paths); start += batch {
		end := start + batch
		if end > len(paths) {
			end = len(paths)
		}

		quoted := make([]string, 0, end-start)
		for _, p := range paths[start:end] {
			quoted = append(quoted, ShellQuote(p))
		}
		args := strings.Join(quoted, " ")
		// The GNU and BSD spellings run back to back with stderr discarded: the
		// one the server has answers, the other prints nothing. Probing first
		// would cost a round trip to learn what the output already tells us.
		cmd := "{ stat -c '%s %Y %n' -- " + args + " 2>/dev/null; " +
			"stat -f '%z %m %N' -- " + args + " 2>/dev/null; } || true"
		out, err := c.RunCommand(cmd)
		if err != nil {
			return nil, fmt.Errorf("remote stat: %w", err)
		}
		for _, line := range strings.Split(out, "\n") {
			name, st, ok := parseStatLine(line)
			if ok {
				stats[name] = st
			}
		}
		if onProgress != nil {
			onProgress(end, len(paths))
		}
	}
	return stats, nil
}

// parseStatLine splits one `stat` output line into its path and metadata. Both
// formats emit "<size> <mtime> <path>", and the path may contain spaces.
func parseStatLine(line string) (name string, st RemoteStat, ok bool) {
	fields := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(fields) != 3 {
		return "", RemoteStat{}, false
	}
	size, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return "", RemoteStat{}, false
	}
	mtime, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return "", RemoteStat{}, false
	}
	if fields[2] == "" {
		return "", RemoteStat{}, false
	}
	return fields[2], RemoteStat{Size: size, ModTime: mtime}, true
}

// remoteBatchSize picks how many paths go into one remote command. Small batches
// while there are few paths, so a caller can report movement; large ones once
// there are thousands, where the round trip dominates and hundreds of them would
// cost more than the work. Every size stays well below the shell's argument limit.
func remoteBatchSize(total int) int {
	switch {
	case total <= 50:
		return 10
	case total <= 500:
		return 50
	default:
		return 200
	}
}

// RemoteSHA256Many hashes paths on the server and returns the lowercase hex
// SHA256 of each one, so only the digests cross the network instead of the file
// contents. Paths that are missing or unreadable are absent from the result
// rather than an error, which is how a caller learns they need uploading.
func (c *Client) RemoteSHA256Many(paths []string, onProgress func(done, total int)) (map[string]string, error) {
	hashes := make(map[string]string, len(paths))
	batch := remoteBatchSize(len(paths))
	for start := 0; start < len(paths); start += batch {
		end := start + batch
		if end > len(paths) {
			end = len(paths)
		}

		quoted := make([]string, 0, end-start)
		for _, p := range paths[start:end] {
			quoted = append(quoted, ShellQuote(p))
		}
		args := strings.Join(quoted, " ")
		// sha256sum exits non-zero when any path is missing but still prints the
		// others, and BSD/macOS servers only ship shasum; `|| true` keeps both
		// cases out of RunCommand's error path.
		cmd := "if command -v sha256sum >/dev/null 2>&1; then sha256sum -- " + args +
			"; else shasum -a 256 -- " + args + "; fi 2>/dev/null || true"
		out, err := c.RunCommand(cmd)
		if err != nil {
			return nil, fmt.Errorf("remote sha256: %w", err)
		}
		for _, line := range strings.Split(out, "\n") {
			hash, name, ok := parseHashLine(line)
			if ok {
				hashes[name] = hash
			}
		}
		if onProgress != nil {
			onProgress(end, len(paths))
		}
	}
	return hashes, nil
}

// parseHashLine splits one `sha256sum` output line into hash and path. The
// leading `*` of binary mode and the `\` GNU coreutils prepends for a path
// holding a backslash or newline are both stripped.
func parseHashLine(line string) (hash, name string, ok bool) {
	line = strings.TrimSuffix(strings.TrimSpace(line), "\r")
	line = strings.TrimPrefix(line, "\\")
	hash, name, found := strings.Cut(line, " ")
	if !found || len(hash) != sha256.Size*2 {
		return "", "", false
	}
	name = strings.TrimPrefix(strings.TrimSpace(name), "*")
	if name == "" {
		return "", "", false
	}
	return strings.ToLower(hash), name, true
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

// RunCommand executes cmd on the remote host through a fresh SSH session
// and returns its stdout. If the command exits non-zero, stderr is wrapped
// into the returned error.
func (c *Client) RunCommand(cmd string) (string, error) {
	sess, err := c.ssh.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer sess.Close()

	var stdout, stderr strings.Builder
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err := sess.Run(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return stdout.String(), fmt.Errorf("%s: %w", msg, err)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// Source identifies which stream a streamed line came from.
type Source int

const (
	SourceStdout Source = iota
	SourceStderr
)

// RunCommandStream executes cmd on a fresh SSH session and delivers each
// stdout/stderr line to onLine as it arrives. It returns the remote exit code
// (0 on success); a non-zero exit is reported via the code, not as an error —
// err is non-nil only for session/transport/timeout failures. ctx cancels the
// session: on ctx.Done the session is closed, which unblocks the readers and
// surfaces as a non-nil error. onLine is never called concurrently.
func (c *Client) RunCommandStream(ctx context.Context, cmd string, onLine func(src Source, line string)) (int, error) {
	sess, err := c.ssh.NewSession()
	if err != nil {
		return -1, fmt.Errorf("ssh session: %w", err)
	}
	defer sess.Close()

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := sess.Start(cmd); err != nil {
		return -1, fmt.Errorf("start command: %w", err)
	}

	// Serialize onLine across both reader goroutines so the callback (and the
	// caller's rendering) never sees interleaved calls.
	var mu sync.Mutex
	emit := func(src Source, line string) {
		mu.Lock()
		onLine(src, line)
		mu.Unlock()
	}

	var wg sync.WaitGroup
	scan := func(r io.Reader, src Source) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			emit(src, sc.Text())
		}
	}
	wg.Add(2)
	go scan(stdout, SourceStdout)
	go scan(stderr, SourceStderr)

	// Close the session when the context is cancelled, unblocking the readers.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			sess.Close()
		case <-done:
		}
	}()

	wg.Wait()
	waitErr := sess.Wait()

	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if waitErr != nil {
		var ee *ssh.ExitError
		if errors.As(waitErr, &ee) {
			return ee.ExitStatus(), nil
		}
		return -1, waitErr
	}
	return 0, nil
}

// DownloadFile copies remotePath from the SFTP server to localPath,
// creating local parent directories as needed. On partial write the
// incomplete local file is removed.
func (c *Client) DownloadFile(remotePath, localPath string) error {
	src, err := c.SFTP.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote %s: %w", remotePath, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(localPath), err)
	}

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local %s: %w", localPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(localPath)
		return fmt.Errorf("download %s: %w", remotePath, err)
	}
	return nil
}

// RunCommandStdin executes cmd on the remote host with stdin piped in and
// returns stdout. Used for `sudo -S` to feed a password without exposing it
// on the command line.
func (c *Client) RunCommandStdin(cmd, stdin string) (string, error) {
	sess, err := c.ssh.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer sess.Close()

	var stdout, stderr strings.Builder
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	sess.Stdin = strings.NewReader(stdin)

	if err := sess.Run(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return stdout.String(), fmt.Errorf("%s: %w", msg, err)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// RemoteWritable reports whether the SSH user can write to dir on the remote.
func (c *Client) RemoteWritable(dir string) (bool, error) {
	out, err := c.RunCommand("[ -w " + ShellQuote(dir) + " ] && echo y || echo n")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "y", nil
}

// ShellQuote wraps s in single quotes safe for POSIX shells.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
