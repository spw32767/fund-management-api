package monitor

import (
	"bufio"
	"os"
	"strconv"
	"strings"

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
      flex-wrap: wrap;
      gap: 1rem;
    }
    
    .logs-title {
      font-size: 1.25rem;
      font-weight: 600;
      color: #a5b4fc;
    }
    
    .logs-controls {
      display: flex;
      gap: 0.5rem;
      align-items: center;
      flex-wrap: wrap;
    }
    
    .logs-info {
      font-size: 0.875rem;
      color: #94a3b8;
      margin-bottom: 1rem;
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
    
    button:disabled {
      opacity: 0.5;
      cursor: not-allowed;
      transform: none;
    }
    
    button.secondary {
      background: linear-gradient(135deg, #64748b 0%, #475569 100%);
      box-shadow: 0 4px 15px rgba(100, 116, 139, 0.3);
    }
    
    button.secondary:hover {
      box-shadow: 0 6px 20px rgba(100, 116, 139, 0.4);
    }
    
    button.danger {
      background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
      box-shadow: 0 4px 15px rgba(239, 68, 68, 0.3);
    }
    
    button.danger:hover {
      box-shadow: 0 6px 20px rgba(239, 68, 68, 0.4);
    }
    
    button.paused {
      background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
      box-shadow: 0 4px 15px rgba(245, 87, 108, 0.3);
    }
    
    button.paused:hover {
      box-shadow: 0 6px 20px rgba(245, 87, 108, 0.4);
    }
    
    select {
      padding: 0.5rem 1rem;
      background: rgba(255, 255, 255, 0.1);
      color: #e0e0e0;
      border: 1px solid rgba(255, 255, 255, 0.2);
      border-radius: 6px;
      font-size: 0.875rem;
    }
    
    input {
      padding: 0.5rem 1rem;
      background: rgba(255, 255, 255, 0.1);
      color: #e0e0e0;
      border: 1px solid rgba(255, 255, 255, 0.2);
      border-radius: 6px;
      font-size: 0.875rem;
      width: 100px;
    }
    
    .modal {
      display: none;
      position: fixed;
      z-index: 1000;
      left: 0;
      top: 0;
      width: 100%;
      height: 100%;
      background-color: rgba(0, 0, 0, 0.7);
      backdrop-filter: blur(5px);
    }
    
    .modal-content {
      background: linear-gradient(135deg, #1e293b 0%, #334155 100%);
      margin: 15% auto;
      padding: 2rem;
      border-radius: 16px;
      width: 90%;
      max-width: 500px;
      box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
      border: 1px solid rgba(255, 255, 255, 0.1);
      text-align: center;
    }
    
    .modal h3 {
      color: #ef4444;
      margin-bottom: 1rem;
      font-size: 1.5rem;
    }
    
    .modal p {
      color: #cbd5e1;
      margin-bottom: 2rem;
      line-height: 1.6;
    }
    
    .modal-buttons {
      display: flex;
      gap: 1rem;
      justify-content: center;
    }
    
    .modal button {
      min-width: 120px;
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
      
      .logs-controls {
        flex-direction: column;
        align-items: stretch;
      }
      
      .logs-controls button,
      .logs-controls select,
      .logs-controls input {
        width: 100%;
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
        <div class="logs-controls">
          <select id="viewMode" onchange="changeViewMode()">
            <option value="tail">Latest (Tail)</option>
            <option value="head">From Start</option>
            <option value="search">Search</option>
          </select>
          <input type="text" id="searchInput" placeholder="Search logs..." style="display:none;" />
          <input type="number" id="linesInput" value="100" min="10" max="1000" title="Lines to show" />
          <button onclick="loadLogs()" class="secondary">🔄 Refresh</button>
          <button onclick="showClearDialog()" class="danger">🗑️ Clear Logs</button>
          <button onclick="toggleLive()" id="toggleBtn">⏸️ Pause Live</button>
        </div>
      </div>
      <div class="logs-info" id="logsInfo">
        <span id="lineCount">Loading...</span>
        <span id="lastUpdate" style="margin-left: 1rem; color: #64748b;"></span>
      </div>
      <pre id="logs" class="loading">Loading logs...</pre>
    </div>
  </div>

  <!-- Clear Logs Modal -->
  <div id="clearModal" class="modal">
    <div class="modal-content">
      <h3>⚠️ Clear Log File</h3>
      <p>
        Are you sure you want to clear the entire log file?<br>
        <strong>This action cannot be undone!</strong><br><br>
        All log history will be permanently deleted.
      </p>
      <div class="modal-buttons">
        <button onclick="hideClearDialog()" class="secondary">Cancel</button>
        <button onclick="confirmClearLogs()" class="danger">Yes, Clear Logs</button>
      </div>
    </div>
  </div>

  <script>
    let liveLogs = true;
    let currentViewMode = 'tail';
    let currentLines = 100;
    let searchTerm = '';
    
    const logsElement = document.getElementById('logs');
    const statusElement = document.getElementById('status');
    const toggleBtn = document.getElementById('toggleBtn');
    const viewModeSelect = document.getElementById('viewMode');
    const searchInput = document.getElementById('searchInput');
    const linesInput = document.getElementById('linesInput');
    const lineCount = document.getElementById('lineCount');
    const lastUpdate = document.getElementById('lastUpdate');
    const clearModal = document.getElementById('clearModal');

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

    function loadLogs() {
      if (!liveLogs && currentViewMode === 'tail') return;
      
      const lines = parseInt(linesInput.value) || 100;
      let url = '/logs?token=secret-token&limit=' + lines;
      
      if (currentViewMode === 'head') {
        url += '&from=start';
      } else if (currentViewMode === 'search' && searchTerm) {
        url += '&search=' + encodeURIComponent(searchTerm);
      }
      
      fetch(url)
        .then(res => res.json())
        .then(data => {
          logsElement.textContent = data.logs;
          logsElement.classList.remove('loading');
          
          // Update info with timestamp
          const info = 'Showing ' + data.count + ' lines';
          const totalInfo = data.total_lines ? ' of ' + data.total_lines.toLocaleString() + ' total lines' : '';
          lineCount.textContent = info + totalInfo;
          
          // Add last update time
          const now = new Date();
          lastUpdate.textContent = 'Last updated: ' + now.toLocaleTimeString();
          
          // Show search results info
          if (currentViewMode === 'search' && searchTerm) {
            lineCount.textContent += ' (filtered by "' + searchTerm + '")';
          }
          
          // Auto scroll for tail mode
          if (currentViewMode === 'tail') {
            logsElement.scrollTop = logsElement.scrollHeight;
          }
        })
        .catch(err => {
          logsElement.textContent = 'Error loading logs: ' + err.message;
          logsElement.classList.remove('loading');
          lineCount.textContent = 'Error loading logs';
          lastUpdate.textContent = '';
        });
    }

    function changeViewMode() {
      currentViewMode = viewModeSelect.value;
      
      if (currentViewMode === 'search') {
        searchInput.style.display = 'block';
        searchInput.addEventListener('input', handleSearch);
      } else {
        searchInput.style.display = 'none';
        searchInput.removeEventListener('input', handleSearch);
      }
      
      // Auto-pause live logs for non-tail modes
      if (currentViewMode !== 'tail' && liveLogs) {
        toggleLive();
      }
      
      loadLogs();
    }

    function handleSearch(e) {
      searchTerm = e.target.value;
      // Debounce search - only search after user stops typing for 300ms
      clearTimeout(window.searchTimeout);
      window.searchTimeout = setTimeout(() => {
        if (searchTerm.length > 2 || searchTerm.length === 0) {
          loadLogs();
        }
      }, 300);
    }

    function toggleLive() {
      liveLogs = !liveLogs;
      toggleBtn.textContent = liveLogs ? '⏸️ Pause Live' : '▶️ Resume Live';
      toggleBtn.classList.toggle('paused', !liveLogs);
      
      if (liveLogs && currentViewMode !== 'tail') {
        viewModeSelect.value = 'tail';
        currentViewMode = 'tail';
        searchInput.style.display = 'none';
      }
    }

    function showClearDialog() {
      clearModal.style.display = 'block';
    }

    function hideClearDialog() {
      clearModal.style.display = 'none';
    }

    function confirmClearLogs() {
      fetch('/logs/clear', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          token: 'secret-token',
          confirm: true
        })
      })
      .then(res => res.json())
      .then(data => {
        if (data.success) {
          logsElement.textContent = 'Log file cleared successfully';
          lineCount.textContent = 'Log file cleared - 0 lines';
          lastUpdate.textContent = 'Cleared at: ' + new Date().toLocaleTimeString();
          
          // Refresh logs after 2 seconds
          setTimeout(() => {
            loadLogs();
          }, 2000);
        } else {
          logsElement.textContent = 'Error clearing logs: ' + (data.error || 'Unknown error');
        }
        hideClearDialog();
      })
      .catch(err => {
        logsElement.textContent = 'Error clearing logs: ' + err.message;
        hideClearDialog();
      });
    }

    // Close modal when clicking outside
    window.onclick = function(event) {
      if (event.target === clearModal) {
        hideClearDialog();
      }
    }

    // Initialize
    linesInput.addEventListener('change', loadLogs);
    
    fetchStatus();
    loadLogs();
    setInterval(fetchStatus, 5000);
    setInterval(() => {
      if (liveLogs && currentViewMode === 'tail') {
        loadLogs();
      }
    }, 5000);
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

		// Get parameters
		limitStr := c.DefaultQuery("limit", "100")
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			limit = 100
		}
		if limit > 1000 {
			limit = 1000 // Max limit for safety
		}

		fromStart := c.Query("from") == "start"
		searchTerm := c.Query("search")

		// Read log file
		file, err := os.Open("fund-api.log")
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to read log"})
			return
		}
		defer file.Close()

		var lines []string
		var totalLines int
		scanner := bufio.NewScanner(file)

		// If searching, collect matching lines
		if searchTerm != "" {
			searchLower := strings.ToLower(searchTerm)
			for scanner.Scan() {
				line := scanner.Text()
				totalLines++
				if strings.Contains(strings.ToLower(line), searchLower) {
					lines = append(lines, line)
					if len(lines) >= limit {
						break
					}
				}
			}
		} else if fromStart {
			// Read from start
			for scanner.Scan() && len(lines) < limit {
				lines = append(lines, scanner.Text())
				totalLines++
			}
			// Count remaining lines if we hit limit
			if len(lines) == limit {
				for scanner.Scan() {
					totalLines++
				}
			}
		} else {
			// Read all lines to get the tail
			allLines := []string{}
			for scanner.Scan() {
				allLines = append(allLines, scanner.Text())
			}
			totalLines = len(allLines)

			// Get last N lines
			start := len(allLines) - limit
			if start < 0 {
				start = 0
			}
			lines = allLines[start:]
		}

		if err := scanner.Err(); err != nil {
			c.JSON(500, gin.H{"error": "Error reading log file"})
			return
		}

		// Join lines and return as JSON
		logContent := strings.Join(lines, "\n")

		c.JSON(200, gin.H{
			"logs":        logContent,
			"count":       len(lines),
			"total_lines": totalLines,
		})
	})

	// Clear logs endpoint
	router.POST("/logs/clear", func(c *gin.Context) {
		const token = "secret-token"

		var request struct {
			Token   string `json:"token"`
			Confirm bool   `json:"confirm"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}

		if request.Token != token {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		if !request.Confirm {
			c.JSON(400, gin.H{"error": "Confirmation required"})
			return
		}

		// Clear the log file by truncating it
		err := os.Truncate("fund-api.log", 0)
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to clear log file"})
			return
		}

		c.JSON(200, gin.H{
			"success": true,
			"message": "Log file cleared successfully",
		})
	})
}
