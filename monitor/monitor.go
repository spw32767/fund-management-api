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
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }
    
    body {
      background: linear-gradient(135deg, #0f0f0f 0%, #1a1a2e 100%);
      color: #e0e0e0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
      min-height: 100vh;
      padding: 20px;
    }
    
    .container {
      max-width: 1200px;
      margin: 0 auto;
    }
    
    h1 {
      font-size: 2.5rem;
      font-weight: 700;
      background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      background-clip: text;
      margin-bottom: 2rem;
      display: flex;
      align-items: center;
      gap: 1rem;
    }
    
    .status-card {
      background: rgba(255, 255, 255, 0.05);
      backdrop-filter: blur(10px);
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 16px;
      padding: 1.5rem;
      margin-bottom: 2rem;
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
      transition: transform 0.3s ease, box-shadow 0.3s ease;
    }
    
    .status-card:hover {
      transform: translateY(-2px);
      box-shadow: 0 12px 40px rgba(102, 126, 234, 0.1);
    }
    
    #status {
      font-size: 1.25rem;
      font-weight: 600;
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    
    .logs-container {
      background: rgba(255, 255, 255, 0.03);
      backdrop-filter: blur(10px);
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 16px;
      padding: 1.5rem;
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
    }
    
    .logs-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 1rem;
      padding-bottom: 1rem;
      border-bottom: 1px solid rgba(255, 255, 255, 0.1);
    }
    
    .logs-title {
      font-size: 1.25rem;
      font-weight: 600;
      color: #a5b4fc;
    }
    
    #logs {
      background: rgba(0, 0, 0, 0.3);
      padding: 1.5rem;
      border-radius: 12px;
      max-height: 500px;
      overflow-y: auto;
      white-space: pre-wrap;
      font-family: 'Monaco', 'Consolas', 'Courier New', monospace;
      font-size: 0.875rem;
      line-height: 1.6;
      color: #cbd5e1;
      scrollbar-width: thin;
      scrollbar-color: rgba(102, 126, 234, 0.5) rgba(255, 255, 255, 0.1);
    }
    
    #logs::-webkit-scrollbar {
      width: 8px;
    }
    
    #logs::-webkit-scrollbar-track {
      background: rgba(255, 255, 255, 0.1);
      border-radius: 4px;
    }
    
    #logs::-webkit-scrollbar-thumb {
      background: rgba(102, 126, 234, 0.5);
      border-radius: 4px;
    }
    
    #logs::-webkit-scrollbar-thumb:hover {
      background: rgba(102, 126, 234, 0.7);
    }
    
    button {
      padding: 0.75rem 1.5rem;
      background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
      color: #ffffff;
      border: none;
      border-radius: 8px;
      cursor: pointer;
      font-weight: 600;
      font-size: 0.875rem;
      transition: all 0.3s ease;
      box-shadow: 0 4px 15px rgba(102, 126, 234, 0.3);
    }
    
    button:hover {
      transform: translateY(-2px);
      box-shadow: 0 6px 20px rgba(102, 126, 234, 0.4);
    }
    
    button:active {
      transform: translateY(0);
    }
    
    button.paused {
      background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
      box-shadow: 0 4px 15px rgba(245, 87, 108, 0.3);
    }
    
    button.paused:hover {
      box-shadow: 0 6px 20px rgba(245, 87, 108, 0.4);
    }
    
    @keyframes pulse {
      0%, 100% {
        opacity: 1;
      }
      50% {
        opacity: 0.5;
      }
    }
    
    .loading {
      animation: pulse 2s ease-in-out infinite;
    }
    
    @media (max-width: 768px) {
      h1 {
        font-size: 2rem;
      }
      
      .status-card, .logs-container {
        padding: 1rem;
      }
      
      #logs {
        max-height: 400px;
        padding: 1rem;
      }
    }
  </style>
</head>
<body>
  <div class="container">
    <h1>🚀 Server Monitor</h1>
    
    <div class="status-card">
      <div class="status" id="status">
        <span class="loading">Status: Checking...</span>
      </div>
    </div>
    
    <div class="logs-container">
      <div class="logs-header">
        <div class="logs-title">📋 Server Logs</div>
        <button onclick="toggleLive()" id="toggleBtn">Pause Live Logs</button>
      </div>
      <pre id="logs" class="loading">Loading logs...</pre>
    </div>
  </div>

  <script>
    let liveLogs = true;
    const logsElement = document.getElementById('logs');
    const statusElement = document.getElementById('status');
    const toggleBtn = document.getElementById('toggleBtn');

    function fetchStatus() {
      fetch('/api/v1/health')
        .then(res => res.json())
        .then(data => {
          statusElement.innerHTML = '<span>Status: ' + (data.success ? '🟢 Online' : '🔴 Offline') + '</span>';
          statusElement.querySelector('span').classList.remove('loading');
        })
        .catch(() => {
          statusElement.innerHTML = '<span>Status: 🔴 Offline</span>';
          statusElement.querySelector('span').classList.remove('loading');
        });
    }

    function fetchLogs() {
      if (!liveLogs) return;
      fetch('/logs?token=secret-token')
        .then(res => res.text())
        .then(data => {
          logsElement.textContent = data;
          logsElement.classList.remove('loading');
          logsElement.scrollTop = logsElement.scrollHeight; // auto scroll
        });
    }

    function toggleLive() {
      liveLogs = !liveLogs;
      toggleBtn.textContent = liveLogs ? 'Pause Live Logs' : 'Resume Live Logs';
      toggleBtn.classList.toggle('paused', !liveLogs);
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
