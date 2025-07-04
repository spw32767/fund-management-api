package monitor

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

const accessToken = "secret-token"

// RegisterMonitorPage sets up the /monitor page
func RegisterMonitorPage(router *gin.Engine) {
	router.GET("/monitor", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, monitorHTML)
	})
}

// RegisterLogsRoute sets up the /logs route
func RegisterLogsRoute(router *gin.Engine) {
	router.GET("/logs", func(c *gin.Context) {
		if c.Query("token") != accessToken {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		logData, err := os.ReadFile("fund-api.log")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read log"})
			return
		}

		c.Data(http.StatusOK, "text/plain; charset=utf-8", logData)
	})
}

const monitorHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>Server Monitor</title>
  <style>
    body {
      background-color: #1e1e1e;
      color: #ffffff;
      font-family: monospace;
      padding: 1rem;
    }
    h1 {
      color: #6cf;
    }
    pre {
      background: #2d2d2d;
      padding: 1rem;
      border-radius: 8px;
      overflow-x: auto;
      max-height: 70vh;
    }
  </style>
</head>
<body>
  <h1>🚀 Server Monitor</h1>
  <p>Status: <span id="status">Checking...</span></p>
  <pre id="log">Loading logs...</pre>

  <script>
    async function updateStatus() {
      try {
        const res = await fetch('/api/v1/health');
        if (res.ok) {
          document.getElementById('status').textContent = '🟢 Online';
        } else {
          document.getElementById('status').textContent = '🔴 Offline';
        }
      } catch {
        document.getElementById('status').textContent = '🔴 Offline';
      }
    }

    async function updateLogs() {
      try {
        const res = await fetch('/logs?token=secret-token');
        const text = await res.text();
        document.getElementById('log').textContent = text;
      } catch {
        document.getElementById('log').textContent = '❌ Failed to fetch logs';
      }
    }

    updateStatus();
    updateLogs();
    setInterval(updateStatus, 10000);
    setInterval(updateLogs, 5000);
  </script>
</body>
</html>
`

// monitor.go
// Package monitor provides a simple monitoring page for the server.
// This HTML provides a simple monitor page that displays the server status and logs.
