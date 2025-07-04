package monitor

import (
	"os"

	"github.com/gin-gonic/gin"
)

func RegisterMonitorPage(router *gin.Engine) {
	router.GET("/monitor", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Server Monitor</title>
  <style>
    body {
      background-color: #1e1e1e;
      color: #ffffff;
      font-family: Arial, sans-serif;
      padding: 20px;
    }
    h1 {
      color: #4fc3f7;
    }
    .status {
      margin-bottom: 10px;
    }
    #logs {
      background-color: #2e2e2e;
      padding: 15px;
      border-radius: 8px;
      max-height: 500px;
      overflow-y: scroll;
      white-space: pre-wrap;
    }
    button {
      margin-top: 10px;
      padding: 6px 14px;
      background-color: #4fc3f7;
      color: #000;
      border: none;
      border-radius: 5px;
      cursor: pointer;
      font-weight: bold;
    }
    button:hover {
      background-color: #29b6f6;
    }
  </style>
</head>
<body>
  <h1>🚀 Server Monitor</h1>
  <div class="status" id="status">Status: Checking...</div>
  <pre id="logs">Loading logs...</pre>
  <button onclick="toggleLive()" id="toggleBtn">Pause Live Logs</button>

  <script>
    let liveLogs = true;
    const logsElement = document.getElementById('logs');
    const statusElement = document.getElementById('status');
    const toggleBtn = document.getElementById('toggleBtn');

    function fetchStatus() {
      fetch('/api/v1/health')
        .then(res => res.json())
        .then(data => {
          statusElement.textContent = 'Status: ' + (data.success ? '🟢 Online' : '🔴 Offline');
        })
        .catch(() => {
          statusElement.textContent = 'Status: 🔴 Offline';
        });
    }

    function fetchLogs() {
      if (!liveLogs) return;
      fetch('/logs?token=secret-token')
        .then(res => res.text())
        .then(data => {
          logsElement.textContent = data;
          logsElement.scrollTop = logsElement.scrollHeight; // auto scroll
        });
    }

    function toggleLive() {
      liveLogs = !liveLogs;
      toggleBtn.textContent = liveLogs ? 'Pause Live Logs' : 'Resume Live Logs';
    }

    fetchStatus();
    fetchLogs();
    setInterval(fetchStatus, 5000);
    setInterval(fetchLogs, 5000);
  </script>
</body>
</html>`))
	})
}

func RegisterLogsRoute(router *gin.Engine) {
	router.GET("/logs", func(c *gin.Context) {
		const token = "secret-token"
		if c.Query("token") != token {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			return
		}
		logData, err := os.ReadFile("fund-api.log")
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to read log"})
			return
		}
		c.Data(200, "text/plain; charset=utf-8", logData)
	})
}
