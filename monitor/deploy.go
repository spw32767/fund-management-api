// deploy.go
package monitor

import (
	"bufio"
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ====== CHANGE THIS TOKEN ======
const deployToken = "secret-deploy" // <- set your own strong value

// Your paths/commands
const (
	repoDir     = "/root/fundproject/fund-management-api"
	buildDir    = "/root/fundproject/fund-management-api/cmd/api"
	outputBin   = "/root/fundproject/fund-api"
	serviceName = "fund-api"

	gitBin       = "/usr/bin/git"
	goBin        = "/usr/local/go/bin/go"
	systemctlBin = "/usr/bin/systemctl"
	sudoBin      = "/usr/bin/sudo"
)

func RegisterDeployPage(router *gin.Engine) {
	router.GET("/deploy", deployUI)
	router.GET("/deploy/preview", deployPreview)
	router.GET("/deploy/diag", deployDiag)
	router.POST("/deploy/run", deployRun)
}

// ---------- UI ----------
func deployUI(c *gin.Context) {
	if c.Query("token") != deployToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	page := `
<!doctype html>
<html>
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>Deploy – Fund API</title>
<style>
  body{font-family:ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial;background:#0b0f14;color:#e5e7eb;margin:0;padding:24px}
  .card{background:#0f172a;border:1px solid #1f2937;border-radius:14px;padding:16px;max-width:900px;margin:auto}
  h1{font-size:20px;margin:0 0 12px}
  .row{display:flex;gap:8px;align-items:center;margin-bottom:12px;flex-wrap:wrap}
  button{padding:8px 14px;border-radius:10px;border:1px solid #334155;background:#111827;color:#e5e7eb;cursor:pointer}
  button:hover{background:#0b1220}
  pre{background:#0b1220;border:1px solid #334155;border-radius:10px;padding:12px;white-space:pre-wrap;max-height:60vh;overflow:auto}
  small{color:#94a3b8}
  .pill{font-size:12px;padding:2px 8px;border-radius:999px;border:1px solid #334155;background:#0b1220;margin-left:8px}
  .danger{background:#8b0000;border-color:#7a0a0a}
  .muted{opacity:.9}
</style>
</head>
<body>
<div class="card">
  <h1>Deploy Latest Backend <span class="pill">repo: {{.RepoDir}}</span></h1>

  <div class="row">
    <button id="btnPreview">Preview Changes</button>
    <button id="btnDiag">Sudo Check</button>
    <button id="btnDeploy" class="danger">Deploy</button>
  </div>

  <pre id="out"><small>Ready.</small></pre>
  <small class="muted">Build Dir: {{.BuildDir}} | Binary: {{.OutputBin}} | Service: {{.ServiceName}}</small>
</div>

<script>
const out = document.getElementById('out');
const token = encodeURIComponent("{{.Token}}");

function set(t){ out.textContent = t; out.scrollTop = out.scrollHeight; }
function append(t){ out.textContent += "\\n"+t; out.scrollTop = out.scrollHeight; }

async function streamPost(url){
  const res = await fetch(url, { method:'POST' });
  if(!res.ok){ throw new Error(await res.text()); }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  while(true){
    const {value, done} = await reader.read();
    if(done) break;
    append(decoder.decode(value, {stream:true}));
  }
}

document.getElementById('btnPreview').onclick = async ()=>{
  set("Fetching and comparing with remote...");
  try{
    const res = await fetch('/deploy/preview?token=' + token);
    set(await res.text());
  }catch(e){ set("ERROR: " + e.message); }
};

document.getElementById('btnDiag').onclick = async ()=>{
  set("Running sudo diagnostics...");
  try{
    const res = await fetch('/deploy/diag?token=' + token);
    set(await res.text());
  }catch(e){ set("ERROR: " + e.message); }
};

document.getElementById('btnDeploy').onclick = async ()=>{
  const ok = confirm(
    "This will:\\n\\n" +
    "1) git pull (merge remote changes)\\n" +
    "2) go build -o {{.OutputBin}}\\n" +
    "3) systemctl restart {{.ServiceName}}\\n\\n" +
    "Are you sure you want to continue?"
  );
  if(!ok) return;

  set("Starting deploy at " + new Date().toLocaleString());
  try{
    await streamPost('/deploy/run?token=' + token);
    append("\\nDone.");
  }catch(e){
    append("\\nERROR: " + e.message);
  }
};
</script>
</body>
</html>`
	tpl, _ := template.New("page").Parse(page)
	_ = tpl.Execute(c.Writer, map[string]any{
		"RepoDir":     repoDir,
		"BuildDir":    buildDir,
		"OutputBin":   outputBin,
		"ServiceName": serviceName,
		"Token":       deployToken,
	})
}

// ---------- PREVIEW ----------
func deployPreview(c *gin.Context) {
	if c.Query("token") != deployToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var buf bytes.Buffer
	w := func(s string) { buf.WriteString(s + "\n") }

	branch, _ := runCmd(repoDir, gitBin, "rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	remoteRef := "origin/" + branch

	w("Preview @ " + time.Now().Format(time.RFC3339))
	w("Repo: " + repoDir)
	w("Current branch: " + branch)
	w("Remote: " + remoteRef)
	w("")

	// Fetch remote updates
	if out, err := runCmd(repoDir, gitBin, "fetch", "--all", "--prune"); err != nil {
		w("ERROR fetching: " + err.Error())
		w(strings.TrimSpace(out))
		c.String(http.StatusOK, buf.String())
		return
	}

	// Warn if local uncommitted changes exist
	if out, _ := runCmd(repoDir, gitBin, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		w("⚠️ Local changes detected (uncommitted):")
		w(strings.TrimSpace(out))
		w("")
	}

	// Incoming commits
	w("> Incoming commits (HEAD.." + remoteRef + "):")
	if out, _ := runCmd(repoDir, gitBin, "log", "--oneline", "HEAD.."+remoteRef); strings.TrimSpace(out) != "" {
		w(out)
	} else {
		w("(none)")
	}
	w("")

	// Files that will change
	w("> Files changed (diff --name-status HEAD.." + remoteRef + "):")
	if out, _ := runCmd(repoDir, gitBin, "diff", "--name-status", "HEAD.."+remoteRef); strings.TrimSpace(out) != "" {
		w(out)
	} else {
		w("(none)")
	}
	w("")

	c.String(http.StatusOK, buf.String())
}

// ---------- DIAGNOSTICS ----------
func deployDiag(c *gin.Context) {
	if c.Query("token") != deployToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var buf bytes.Buffer
	w := func(s string) { buf.WriteString(s + "\n") }

	w("Deploy Diagnostics @ " + time.Now().Format(time.RFC3339))
	w("User: " + whoami())
	w("RepoDir: " + repoDir)
	w("BuildDir: " + buildDir)
	if p, _ := filepath.Abs(outputBin); p != "" {
		w("Binary: " + p)
	}
	w("Service: " + serviceName)
	w("")

	// Dir checks
	if _, err := os.Stat(repoDir); err != nil {
		w("WARN: repoDir not accessible: " + err.Error())
	}
	if _, err := os.Stat(buildDir); err != nil {
		w("WARN: buildDir not accessible: " + err.Error())
	}

	// sudo non-interactive test
	w("\n> sudo -n -v")
	if out, err := runCmd("", sudoBin, "-n", "-v"); err != nil {
		w("ERROR: " + err.Error())
		w(strings.TrimSpace(out))
	} else {
		w("OK")
	}

	// sudo systemctl status
	w("\n> sudo -n systemctl status " + serviceName)
	if out, err := runCmd("", sudoBin, "-n", systemctlBin, "status", serviceName); err != nil {
		w("ERROR: " + err.Error())
		w(strings.TrimSpace(out))
	} else {
		w("OK")
	}

	// active?
	w("\n> systemctl is-active " + serviceName)
	if out, err := runCmd("", systemctlBin, "is-active", serviceName); err != nil {
		w("ERROR: " + err.Error())
		w(strings.TrimSpace(out))
	} else {
		w(strings.TrimSpace(out))
	}

	c.String(http.StatusOK, buf.String())
}

// ---------- DEPLOY (via login shell) ----------
func deployRun(c *gin.Context) {
	if c.Query("token") != deployToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Streaming response
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.String(http.StatusInternalServerError, "streaming not supported by server")
		return
	}
	write := func(s string) {
		_, _ = c.Writer.Write([]byte(s + "\n"))
		flusher.Flush()
	}

	write("== Deploy via login shell ==")

	// Use a login shell so it behaves the same as your terminal session.
	// Absolute paths are kept for extra safety.
	cmd := exec.Command("/bin/bash", "-lc", `
set -e
cd /root/fundproject/fund-management-api
`+gitBin+` pull
cd cmd/api
`+goBin+` build -o `+outputBin+`
`+sudoBin+` -n `+systemctlBin+` restart `+serviceName+` || `+systemctlBin+` restart `+serviceName+`
`)

	if err := runCmdStreaming(cmd, write); err != nil {
		write("ERROR: deploy failed: " + err.Error())
		return
	}

	// Show status after restart
	write("\n== Status after restart ==")
	_ = runAndStream(write, "", systemctlBin, "is-active", serviceName)
}

// ---------- helpers ----------
func runAndStream(write func(string), dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	outScan := bufio.NewScanner(stdout)
	errScan := bufio.NewScanner(stderr)
	outScan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	errScan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	done := make(chan struct{}, 2)
	go func() {
		for outScan.Scan() {
			write(outScan.Text())
		}
		done <- struct{}{}
	}()
	go func() {
		for errScan.Scan() {
			write(errScan.Text())
		}
		done <- struct{}{}
	}()
	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		return errors.New(name + " failed: " + err.Error())
	}
	return nil
}

func runCmdStreaming(cmd *exec.Cmd, write func(string)) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	outScan := bufio.NewScanner(stdout)
	errScan := bufio.NewScanner(stderr)
	outScan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	errScan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	done := make(chan struct{}, 2)
	go func() {
		for outScan.Scan() {
			write(outScan.Text())
		}
		done <- struct{}{}
	}()
	go func() {
		for errScan.Scan() {
			write(errScan.Text())
		}
		done <- struct{}{}
	}()
	<-done
	<-done

	return cmd.Wait()
}

func runCmd(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func whoami() string {
	out, err := runCmd("", "id", "-u", "-n")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(out)
}
