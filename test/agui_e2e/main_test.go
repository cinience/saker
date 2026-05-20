//go:build agui_e2e

package agui_e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

var serverURL string

func TestMain(m *testing.M) {
	if os.Getenv("DASHSCOPE_API_KEY") == "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		fmt.Println("SKIP: agui_e2e requires DASHSCOPE_API_KEY or ANTHROPIC_API_KEY")
		os.Exit(0)
	}

	binary, err := buildServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build server: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(binary)

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find free port: %v\n", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf(":%d", port)
	serverURL = fmt.Sprintf("http://localhost:%d", port)

	cmd := exec.Command(binary, "--server", "--server-addr", addr)
	cmd.Env = append(os.Environ(), "SAKER_DEV_BYPASS_AUTH=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start server: %v\n", err)
		os.Exit(1)
	}

	if err := waitForServer(serverURL, 30*time.Second); err != nil {
		_ = cmd.Process.Kill()
		fmt.Fprintf(os.Stderr, "server not ready: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	os.Exit(code)
}

func buildServer() (string, error) {
	repoRoot := findRepoRoot()
	binary := filepath.Join(os.TempDir(), fmt.Sprintf("saker-e2e-test-%d", os.Getpid()))
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	// Ensure frontend dist dirs exist (server embeds them).
	for _, dir := range []string{"pkg/cli/frontend/dist", "pkg/cli/editor/dist"} {
		d := filepath.Join(repoRoot, dir)
		os.MkdirAll(d, 0o755)
		placeholder := filepath.Join(d, ".gitkeep")
		if _, err := os.Stat(placeholder); os.IsNotExist(err) {
			os.WriteFile(placeholder, nil, 0o644)
		}
	}

	cmd := exec.Command("go", "build", "-o", binary, "./cmd/saker")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build: %s: %w", string(out), err)
	}
	return binary, nil
}

func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

func waitForServer(baseURL string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	infoURL := baseURL + "/v1/agents/run/info"
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for server at %s", infoURL)
		default:
		}
		resp, err := http.Get(infoURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}
