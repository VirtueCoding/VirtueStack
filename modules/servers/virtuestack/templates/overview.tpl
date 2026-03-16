{**
 * VirtueStack WHMCS Module - Client Area Overview Template
 *
 * Displays VM details and embedded Customer WebUI with SSO.
 * Shows provisioning status for async operations.
 *
 * @package VirtueStack\WHMCS
 * @author  VirtueStack Team
 *}

{if $status eq 'provisioning'}
    <div class="virtuestack-provisioning">
        <div class="alert alert-info">
            <h4><i class="fas fa-spinner fa-spin"></i> Your VPS is being provisioned</h4>
            <p>Your virtual server is currently being created. This usually takes 30-60 seconds.</p>
            {if $task_id}
            <p><strong>Task ID:</strong> <code>{$task_id|escape:'htmlall'}</code></p>
            {/if}
        </div>
        
        <div class="provisioning-progress">
            <div class="progress">
                <div class="progress-bar progress-bar-striped active" role="progressbar" style="width: 100%;">
                    <span>Provisioning in progress...</span>
                </div>
            </div>
        </div>
        
        <p class="text-muted">
            <small>This page will automatically refresh. Please wait...</small>
        </p>
        
        <script type="text/javascript">
            // Auto-refresh every 5 seconds while provisioning
            setTimeout(function() {
                location.reload();
            }, 5000);
        </script>
    </div>

{elseif $status eq 'error'}
    <div class="virtuestack-error">
        <div class="alert alert-danger">
            <h4><i class="fas fa-exclamation-triangle"></i> Error</h4>
            <p>{$error|escape:'htmlall'}</p>
        </div>
    </div>

{elseif $status eq 'suspended'}
    <div class="virtuestack-suspended">
        <div class="alert alert-warning">
            <h4><i class="fas fa-pause-circle"></i> Service Suspended</h4>
            <p>Your VPS has been suspended. Please contact support for assistance.</p>
        </div>
        
        <div class="vm-details panel panel-default">
            <div class="panel-heading">
                <h3 class="panel-title">Service Details</h3>
            </div>
            <div class="panel-body">
                <table class="table table-striped">
                    <tr>
                        <th>VM ID</th>
                        <td><code>{$vm_id|escape:'htmlall'}</code></td>
                    </tr>
                    {if $vm_ip}
                    <tr>
                        <th>IP Address</th>
                        <td>{$vm_ip|escape:'htmlall'}</td>
                    </tr>
                    {/if}
                    <tr>
                        <th>Status</th>
                        <td><span class="label label-warning">Suspended</span></td>
                    </tr>
                </table>
            </div>
        </div>
    </div>

{else}
    {* Main VM Management Interface *}
    {if $iframe_url}
    <div class="virtuestack-webui-container">
        <div class="webui-header">
            <div class="row">
                <div class="col-md-8">
                    <h3>
                        <i class="fas fa-server"></i> VPS Management
                        {if $vm_ip}
                        <small class="text-muted">{$vm_ip|escape:'htmlall'}</small>
                        {/if}
                    </h3>
                </div>
                <div class="col-md-4 text-right">
                    <div class="btn-group" role="group">
                        <form method="POST" action="clientarea.php?action=productdetails&id={$service_id|escape:'htmlall'}&modop=custom&a=startVM" style="display:inline;">
                            <input type="hidden" name="token" value="{$token}">
                            <button type="submit" class="btn btn-success btn-sm" title="Start VM">
                                <i class="fas fa-play"></i> Start
                            </button>
                        </form>
                        <form method="POST" action="clientarea.php?action=productdetails&id={$service_id|escape:'htmlall'}&modop=custom&a=stopVM" style="display:inline;" onsubmit="return confirm('Are you sure you want to stop this VM?');">
                            <input type="hidden" name="token" value="{$token}">
                            <button type="submit" class="btn btn-warning btn-sm" title="Stop VM">
                                <i class="fas fa-stop"></i> Stop
                            </button>
                        </form>
                        <form method="POST" action="clientarea.php?action=productdetails&id={$service_id|escape:'htmlall'}&modop=custom&a=restartVM" style="display:inline;" onsubmit="return confirm('Are you sure you want to restart this VM?');">
                            <input type="hidden" name="token" value="{$token}">
                            <button type="submit" class="btn btn-info btn-sm" title="Restart VM">
                                <i class="fas fa-redo"></i> Restart
                            </button>
                        </form>
                    </div>
                </div>
            </div>
        </div>
        
        <div class="webui-iframe-wrapper">
            <iframe 
                src="{$iframe_url|escape:'htmlall'}" 
                id="virtuestack-webui-frame"
                class="virtuestack-iframe"
                sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
                allow="clipboard-read; clipboard-write"
                loading="lazy"
                referrerpolicy="strict-origin-when-cross-origin"
            >
                <p>Your browser does not support iframes. Please use a modern browser.</p>
            </iframe>
        </div>
        
        {* Console Quick Links *}
        <div class="console-links">
            <div class="btn-group" role="group">
                {if $console_url}
                <a href="{$console_url|escape:'htmlall'}" target="_blank" class="btn btn-default btn-sm">
                    <i class="fas fa-desktop"></i> VNC Console
                </a>
                {/if}
                <a href="#" onclick="openConsole('vnc')" class="btn btn-default btn-sm">
                    <i class="fas fa-tv"></i> Open VNC
                </a>
                <a href="#" onclick="openConsole('serial')" class="btn btn-default btn-sm">
                    <i class="fas fa-terminal"></i> Serial Console
                </a>
            </div>
        </div>
    </div>
    
    <style type="text/css">
        .virtuestack-webui-container {
            margin: 20px 0;
        }
        
        .webui-header {
            background: #f5f5f5;
            padding: 15px 20px;
            border: 1px solid #ddd;
            border-bottom: none;
            border-radius: 4px 4px 0 0;
        }
        
        .webui-header h3 {
            margin: 0;
            font-size: 1.3em;
        }
        
        .webui-header h3 small {
            font-size: 0.7em;
            margin-left: 10px;
        }
        
        .webui-iframe-wrapper {
            position: relative;
            padding-bottom: 75%;
            height: 0;
            overflow: hidden;
            border: 1px solid #ddd;
            border-radius: 0 0 4px 4px;
            background: #fff;
        }
        
        .virtuestack-iframe {
            position: absolute;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            border: none;
        }
        
        .console-links {
            margin-top: 15px;
            text-align: center;
        }
        
        @media (max-width: 768px) {
            .webui-header .btn-group {
                margin-top: 10px;
            }
            
            .webui-iframe-wrapper {
                padding-bottom: 100%;
            }
        }
        
        /* Dark mode support */
        @media (prefers-color-scheme: dark) {
            .webui-header {
                background: #2a2a2a;
                border-color: #444;
            }
            
            .webui-iframe-wrapper {
                background: #1a1a1a;
                border-color: #444;
            }
        }
    </style>
    
    <script type="text/javascript">
        function openConsole(type) {
            var iframe = document.getElementById('virtuestack-webui-frame');
            if (iframe && iframe.contentWindow) {
                iframe.contentWindow.postMessage({
                    action: 'openConsole',
                    type: type
                }, document.location.origin);
            }
        }
    </script>
    
    {else}
    {* Fallback if no iframe URL available *}
    <div class="virtuestack-details">
        <div class="panel panel-default">
            <div class="panel-heading">
                <h3 class="panel-title">VPS Details</h3>
            </div>
            <div class="panel-body">
                <table class="table table-striped">
                    <tr>
                        <th>VM ID</th>
                        <td><code>{$vm_id|escape:'htmlall'}</code></td>
                    </tr>
                    {if $vm_ip}
                    <tr>
                        <th>IP Address</th>
                        <td>{$vm_ip|escape:'htmlall'}</td>
                    </tr>
                    {/if}
                    <tr>
                        <th>Status</th>
                        <td>
                            {if $status eq 'running'}
                            <span class="label label-success">Running</span>
                            {elseif $status eq 'stopped'}
                            <span class="label label-default">Stopped</span>
                            {elseif $status eq 'suspended'}
                            <span class="label label-warning">Suspended</span>
                            {else}
                            <span class="label label-info">{$status|escape:'htmlall'|ucfirst}</span>
                            {/if}
                        </td>
                    </tr>
                </table>
                
                <p class="text-muted">
                    <i class="fas fa-info-circle"></i> 
                    The management panel is not available. Please contact support for assistance.
                </p>
            </div>
        </div>
    </div>
    {/if}
{/if}

{* Additional VM Information Card *}
{if $vm_id && $status neq 'provisioning' && $status neq 'error'}
<div class="virtuestack-info-card" style="margin-top: 20px;">
    <div class="panel panel-info">
        <div class="panel-heading">
            <h3 class="panel-title"><i class="fas fa-info-circle"></i> Quick Actions</h3>
        </div>
        <div class="panel-body">
            <div class="row">
                <div class="col-md-6">
                    <h4>Console Access</h4>
                    <p>Access your VPS directly through the web-based console.</p>
                    <p>
                        <form method="POST" action="clientarea.php?action=productdetails&id={$service_id|escape:'htmlall'}&modop=custom&a=openConsole&type=vnc" style="display:inline;">
                            <input type="hidden" name="token" value="{$token}">
                            <button type="submit" class="btn btn-primary">
                                <i class="fas fa-desktop"></i> Open VNC Console
                            </button>
                        </form>
                    </p>
                </div>
                <div class="col-md-6">
                    <h4>Power Control</h4>
                    <p>Control the power state of your virtual server.</p>
                    <div class="btn-group">
                        <form method="POST" action="clientarea.php?action=productdetails&id={$service_id|escape:'htmlall'}&modop=custom&a=startVM" style="display:inline;">
                            <input type="hidden" name="token" value="{$token}">
                            <button type="submit" class="btn btn-success">
                                <i class="fas fa-play"></i> Start
                            </button>
                        </form>
                        <form method="POST" action="clientarea.php?action=productdetails&id={$service_id|escape:'htmlall'}&modop=custom&a=stopVM" style="display:inline;" onsubmit="return confirm('Are you sure you want to stop this VM?');">
                            <input type="hidden" name="token" value="{$token}">
                            <button type="submit" class="btn btn-warning">
                                <i class="fas fa-stop"></i> Stop
                            </button>
                        </form>
                        <form method="POST" action="clientarea.php?action=productdetails&id={$service_id|escape:'htmlall'}&modop=custom&a=restartVM" style="display:inline;" onsubmit="return confirm('Are you sure you want to restart this VM?');">
                            <input type="hidden" name="token" value="{$token}">
                            <button type="submit" class="btn btn-info">
                                <i class="fas fa-redo"></i> Restart
                            </button>
                        </form>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
{/if}
