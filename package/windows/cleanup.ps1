<#
    clean all windows settings
#>

$ErrorActionPreference = 'Stop'
$WarningPreference = 'SilentlyContinue'
$VerbosePreference = 'SilentlyContinue'
$DebugPreference = 'SilentlyContinue'
$InformationPreference = 'SilentlyContinue'

Import-Module -WarningAction Ignore -Name "$PSScriptRoot\utils.psm1"

function Get-VmComputeNativeMethods()
{
    $ret = 'VmCompute.PrivatePInvoke.NativeMethods' -as [type]
    if (-not $ret) {
        $signature = @'
[DllImport("vmcompute.dll")]
public static extern void HNSCall([MarshalAs(UnmanagedType.LPWStr)] string method, [MarshalAs(UnmanagedType.LPWStr)] string path, [MarshalAs(UnmanagedType.LPWStr)] string request, [MarshalAs(UnmanagedType.LPWStr)] out string response);
'@
        $ret = Add-Type -MemberDefinition $signature -Namespace VmCompute.PrivatePInvoke -Name "NativeMethods" -PassThru
    }
    return $ret
}

function Invoke-HNSRequest
{
    param
    (
        [ValidateSet('GET', 'POST', 'DELETE')]
        [parameter(Mandatory = $true)] [string] $Method,
        [ValidateSet('networks', 'endpoints', 'activities', 'policylists', 'endpointstats', 'plugins')]
        [parameter(Mandatory = $true)] [string] $Type,
        [parameter(Mandatory = $false)] [string] $Action,
        [parameter(Mandatory = $false)] [string] $Data = "",
        [parameter(Mandatory = $false)] [Guid] $Id = [Guid]::Empty
    )

    $hnsPath = "/$Type"
    if ($id -ne [Guid]::Empty) {
        $hnsPath += "/$id"
    }
    if ($Action) {
        $hnsPath += "/$Action"
    }

    $response = ""
    $hnsApi = Get-VmComputeNativeMethods
    $hnsApi::HNSCall($Method, $hnsPath, "$Data", [ref]$response)

    $output = @()
    if ($response) {
        try {
            $output = ($response | ConvertFrom-Json)
            if ($output.Error) {
                Log-Error $output;
            } else {
                $output = $output.Output;
            }
        } catch {
            Log-Error $_.Exception.Message
        }
    }

    return $output;
}

Log-Info "Start cleanning ..."

# check identity
if (-not (Is-Administrator))
{
    Log-Fatal "You need elevated Administrator privileges in order to run this script, start Windows PowerShell by using the Run as Administrator option"
}

# clean up docker conatiner: docker rm -fv $(docker ps -qa)
$containers = $(docker.exe ps -aq)
if ($containers)
{
    Log-Info "Cleaning up docker containers ..."
    $errMsg = $($containers | ForEach-Object {docker.exe rm -f $_})
    if (-not $?) {
        Log-Warn "Could not remove docker containers: $errMsg"
    }

    # wati a while for rancher-wins to clean up processes
    Start-Sleep -Seconds 10
}

# clean up kubernetes components processes
Get-Process -ErrorAction Ignore -Name "rancher-wins-*" | ForEach-Object {
    Log-Info "Stopping process $($_.Name) ..."
    $_ | Stop-Process -ErrorAction Ignore -Force
}

# clean up firewall rules
Get-NetFirewallRule -PolicyStore ActiveStore -Name "rancher-wins-*" -ErrorAction Ignore | ForEach-Object {
    Log-Info "Cleaning up firewall rule $($_.Name) ..."
    $_ | Remove-NetFirewallRule -ErrorAction Ignore | Out-Null
}

# clean up rancher-wins service
Get-Service -Name "rancher-wins" -ErrorAction Ignore | Where-Object {$_.Status -eq "Running"} | ForEach-Object {
    Log-Info "Stopping rancher-wins service ..."
    $_ | Stop-Service -Force -ErrorAction Ignore

    Log-Info "Unregistering rancher-wins service ..."
    Push-Location c:\etc\rancher
    $errMsg = $(.\wins.exe srv app run --unregister)
    if (-not $?) {
        Log-Warn "Could not unregister: $errMsg"
    }
    Pop-Location
}

# clean up network settings
try {
    Invoke-HNSRequest -Method "GET" -Type "networks" | Where-Object {@('cbr0', 'vxlan0') -contains $_.Name} | ForEach-Object {
        Log-Info "Cleaning up HNSNetwork $($_.Name) ..."
        Invoke-HNSRequest -Method "DELETE" -Type "networks" -Id $_.Id
    }

    Invoke-HNSRequest -Method "GET" -Type "policylists" | ForEach-Object {
        Log-Info "Cleaning up HNSPolicyList $($_.Id) ..."
        Invoke-HNSRequest -Method "DELETE" -Type "policylists" -Id $_.Id
    }
} catch {
    throw $_
    Log-Warn "Could not clean: $($_.Exception.Message)"
}

# clean up data
Get-Item -ErrorAction Ignore -Path @(
    "c:\run\*"
    "c:\opt\*"
    "c:\etc\rancher\*"
    "c:\etc\nginx\*"
    "c:\etc\cni\*"
    "c:\etc\kubernetes\*"
    "c:\var\run\*"
    "c:\var\log\*"
    "c:\var\lib\*"
) | ForEach-Object {
    Log-Info "Cleaning up data $($_.FullName) ..."
    try {
        $_ | Remove-Item -ErrorAction Ignore -Recurse -Force | Out-Null
    } catch {
        Log-Warn "Could not clean: $($_.Exception.Message)"
    }
}

Log-Info "Finished!!!"
