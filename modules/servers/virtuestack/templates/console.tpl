{**
 * VirtueStack WHMCS Module - Console Template
 *
 * Full-screen console access via iframe embedding Customer WebUI console.
 * Supports both VNC (graphical) and Serial (text) console types.
 *
 * @package VirtueStack\WHMCS
 * @author  VirtueStack Team
 *}

{* Check console type *}
{assign var="console_type" value=$smarty.get.type|default:'vnc'}

<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <title>{if $console_type eq 'serial'}Serial{else}VNC{/if} Console - {$vm_id|escape:'htmlall'}</title>
    
    <style type="text/css">
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        html, body {
            height: 100%;
            width: 100%;
            overflow: hidden;
            background: #1a1a2e;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
        }
        
        .console-header {
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            height: 50px;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            border-bottom: 1px solid #0f3460;
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 0 20px;
            z-index: 1000;
        }
        
        .console-title {
            color: #e94560;
            font-size: 16px;
            font-weight: 600;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .console-title i {
            font-size: 20px;
        }
        
        .console-info {
            color: #a0a0a0;
            font-size: 13px;
        }
        
        .console-info code {
            background: #0f3460;
            padding: 2px 6px;
            border-radius: 3px;
            color: #00d9ff;
        }
        
        .console-controls {
            display: flex;
            align-items: center;
            gap: 15px;
        }
        
        .btn-console {
            background: #0f3460;
            color: #fff;
            border: 1px solid #1a3a5c;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 13px;
            transition: all 0.2s;
            text-decoration: none;
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        
        .btn-console:hover {
            background: #1a3a5c;
            border-color: #e94560;
        }
        
        .btn-console.active {
            background: #e94560;
            border-color: #e94560;
        }
        
        .btn-danger {
            background: #dc3545;
            border-color: #dc3545;
        }
        
        .btn-danger:hover {
            background: #c82333;
        }
        
        .console-container {
            position: fixed;
            top: 50px;
            left: 0;
            right: 0;
            bottom: 0;
            background: #000;
        }
        
        .console-iframe {
            width: 100%;
            height: 100%;
            border: none;
            background: #000;
        }
        
        .console-loading {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            text-align: center;
            color: #fff;
        }
        
        .console-loading .spinner {
            width: 50px;
            height: 50px;
            border: 3px solid #0f3460;
            border-top-color: #e94560;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin: 0 auto 20px;
        }
        
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        
        .console-status {
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: rgba(26, 26, 46, 0.9);
            border: 1px solid #0f3460;
            border-radius: 8px;
            padding: 10px 15px;
            color: #fff;
            font-size: 12px;
            z-index: 1000;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .status-indicator {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            background: #28a745;
            animation: pulse 2s infinite;
        }
        
        .status-indicator.connecting {
            background: #ffc107;
        }
        
        .status-indicator.disconnected {
            background: #dc3545;
            animation: none;
        }
        
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        
        .fullscreen-mode .console-header {
            opacity: 0;
            transition: opacity 0.3s;
        }
        
        .fullscreen-mode .console-header:hover {
            opacity: 1;
        }
        
        .fullscreen-mode .console-container {
            top: 0;
        }
        
        .error-message {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            text-align: center;
            color: #e94560;
            background: #1a1a2e;
            padding: 40px;
            border-radius: 8px;
            border: 1px solid #0f3460;
        }
        
        .error-message h2 {
            margin-bottom: 15px;
        }
        
        .error-message p {
            color: #a0a0a0;
            margin-bottom: 20px;
        }
        
        /* Clipboard paste modal */
        .paste-modal {
            display: none;
            position: fixed;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            background: #1a1a2e;
            border: 1px solid #0f3460;
            border-radius: 8px;
            padding: 20px;
            z-index: 2000;
            width: 90%;
            max-width: 500px;
        }
        
        .paste-modal.show {
            display: block;
        }
        
        .paste-modal h3 {
            color: #fff;
            margin-bottom: 15px;
        }
        
        .paste-modal textarea {
            width: 100%;
            height: 150px;
            background: #000;
            border: 1px solid #0f3460;
            color: #fff;
            padding: 10px;
            border-radius: 4px;
            resize: none;
            font-family: monospace;
        }
        
        .paste-modal-buttons {
            margin-top: 15px;
            text-align: right;
        }
        
        .modal-overlay {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.7);
            z-index: 1500;
        }
        
        .modal-overlay.show {
            display: block;
        }
    </style>
    
    {* Font Awesome for icons *}
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.4.0/css/all.min.css">
</head>
<body>
    <div class="console-header">
        <div class="console-title">
            <i class="fas fa-{if $console_type eq 'serial'}terminal{else}desktop{/if}"></i>
            <span>{if $console_type eq 'serial'}Serial{else}VNC{/if} Console</span>
            <span class="console-info">
                VM: {if $vm_ip}<code>{$vm_ip|escape:'htmlall'}</code>{else}<code>{$vm_id|escape:'htmlall'|truncate:8:''}</code>{/if}
            </span>
        </div>
        
        <div class="console-controls">
            {* Console Type Switcher *}
            <div class="btn-group">
                <a href="?type=vnc" class="btn-console {if $console_type eq 'vnc'}active{/if}">
                    <i class="fas fa-desktop"></i> VNC
                </a>
                <a href="?type=serial" class="btn-console {if $console_type eq 'serial'}active{/if}">
                    <i class="fas fa-terminal"></i> Serial
                </a>
            </div>
            
            {* Control Buttons *}
            <button class="btn-console" onclick="sendCtrlAltDel()" title="Send Ctrl+Alt+Delete">
                <i class="fas fa-keyboard"></i> Ctrl+Alt+Del
            </button>
            
            <button class="btn-console" onclick="toggleFullscreen()" title="Toggle Fullscreen">
                <i class="fas fa-expand"></i>
            </button>
            
            <button class="btn-console" onclick="showPasteModal()" title="Paste Text">
                <i class="fas fa-paste"></i>
            </button>
            
            <a href="javascript:window.close()" class="btn-console btn-danger" title="Close Console">
                <i class="fas fa-times"></i> Close
            </a>
        </div>
    </div>
    
    <div class="console-container">
        <div id="console-loading" class="console-loading">
            <div class="spinner"></div>
            <p>Connecting to {if $console_type eq 'serial'}Serial{else}VNC{/if} Console...</p>
        </div>
        
        {if $iframe_url}
        <iframe 
            id="console-iframe"
            class="console-iframe"
            src="{$iframe_url|escape:'htmlall'}"
            sandbox="allow-scripts allow-same-origin allow-forms"
            allow="clipboard-read; clipboard-write"
            loading="eager"
            referrerpolicy="strict-origin-when-cross-origin"
            onload="consoleLoaded()"
        ></iframe>
        {else}
        <div class="error-message">
            <h2><i class="fas fa-exclamation-triangle"></i> Console Unavailable</h2>
            <p>The console could not be initialized. Please ensure your VM is running and try again.</p>
            <a href="javascript:window.close()" class="btn-console">Close Window</a>
        </div>
        {/if}
    </div>
    
    <div class="console-status">
        <div id="status-indicator" class="status-indicator connecting"></div>
        <span id="status-text">Connecting...</span>
    </div>
    
    {* Paste Modal *}
    <div id="modal-overlay" class="modal-overlay" onclick="hidePasteModal()"></div>
    <div id="paste-modal" class="paste-modal">
        <h3><i class="fas fa-paste"></i> Paste Text to Console</h3>
        <textarea id="paste-text" placeholder="Paste your text here..."></textarea>
        <div class="paste-modal-buttons">
            <button class="btn-console" onclick="hidePasteModal()">Cancel</button>
            <button class="btn-console" onclick="sendPaste()" style="background: #e94560; margin-left: 10px;">
                <i class="fas fa-paper-plane"></i> Send
            </button>
        </div>
    </div>
    
    <script type="text/javascript">
        var consoleType = '{$console_type|escape:'javascript'}';
        var consoleIframe = document.getElementById('console-iframe');
        var isLoading = true;
        var connectionStatus = 'connecting';
        
        // Console loaded callback
        function consoleLoaded() {
            isLoading = false;
            updateStatus('connected');
            document.getElementById('console-loading').style.display = 'none';
        }
        
        // Update connection status
        function updateStatus(status, text) {
            var indicator = document.getElementById('status-indicator');
            var statusText = document.getElementById('status-text');
            
            indicator.className = 'status-indicator ' + status;
            
            if (text) {
                statusText.textContent = text;
            } else {
                switch(status) {
                    case 'connected':
                        statusText.textContent = 'Connected';
                        break;
                    case 'connecting':
                        statusText.textContent = 'Connecting...';
                        break;
                    case 'disconnected':
                        statusText.textContent = 'Disconnected';
                        break;
                }
            }
        }
        
        // Send Ctrl+Alt+Delete to VNC console
        function sendCtrlAltDel() {
            if (consoleType === 'vnc' && consoleIframe && consoleIframe.contentWindow) {
                consoleIframe.contentWindow.postMessage({
                    type: 'virtuestack:ctrlaltdel'
                }, document.location.origin);
            }
        }
        
        // Toggle fullscreen mode
        function toggleFullscreen() {
            if (!document.fullscreenElement) {
                document.documentElement.requestFullscreen().catch(function() {});
                document.body.classList.add('fullscreen-mode');
            } else {
                document.exitFullscreen();
                document.body.classList.remove('fullscreen-mode');
            }
        }
        
        // Show paste modal
        function showPasteModal() {
            document.getElementById('modal-overlay').classList.add('show');
            document.getElementById('paste-modal').classList.add('show');
            document.getElementById('paste-text').focus();
        }
        
        // Hide paste modal
        function hidePasteModal() {
            document.getElementById('modal-overlay').classList.remove('show');
            document.getElementById('paste-modal').classList.remove('show');
        }
        
        // Send pasted text to console
        function sendPaste() {
            var text = document.getElementById('paste-text').value;
            if (text && consoleIframe && consoleIframe.contentWindow) {
                consoleIframe.contentWindow.postMessage({
                    type: 'virtuestack:paste',
                    text: text
                }, document.location.origin);
                hidePasteModal();
                document.getElementById('paste-text').value = '';
            }
        }
        
        // Handle keyboard shortcuts
        document.addEventListener('keydown', function(e) {
            // F11 for fullscreen
            if (e.key === 'F11') {
                e.preventDefault();
                toggleFullscreen();
            }
            
            // Escape to close modal
            if (e.key === 'Escape') {
                hidePasteModal();
            }
            
            // Ctrl+Shift+V for paste
            if (e.ctrlKey && e.shiftKey && e.key === 'V') {
                e.preventDefault();
                showPasteModal();
            }
        });
        
        // Listen for messages from iframe
        window.addEventListener('message', function(event) {
            // Validate origin
            if (!event.origin.endsWith('{$webui_url|parse_url:$smarty.const.PHP_URL_HOST|default:""}')) {
                return;
            }
            
            if (event.data && event.data.type) {
                switch(event.data.type) {
                    case 'virtuestack:status':
                        updateStatus(event.data.status, event.data.message);
                        break;
                    case 'virtuestack:disconnected':
                        updateStatus('disconnected', 'Connection lost');
                        break;
                }
            }
        });
        
        // Connection timeout
        setTimeout(function() {
            if (isLoading) {
                updateStatus('disconnected', 'Connection timeout');
            }
        }, 30000);
        
        // Warn before leaving
        window.addEventListener('beforeunload', function(e) {
            e.preventDefault();
            e.returnValue = '';
        });
    </script>
</body>
</html>
