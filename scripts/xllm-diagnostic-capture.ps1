[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet('enable', 'disable', 'status', 'export')]
    [string]$Action,

    [string]$Name = 'targeted-capture',
    [string[]]$Model = @(),
    [string[]]$UpstreamModel = @(),
    [int[]]$Channel = @(),
    [int[]]$UserId = @(),
    [int[]]$TokenId = @(),
    [string[]]$Group = @(),
    [string[]]$Protocol = @(),
    [ValidateRange(1, 168)]
    [int]$Hours = 2,
    [ValidateRange(0.000001, 1.0)]
    [double]$SampleRate = 1.0,
    [ValidateRange(1, 1000000)]
    [int]$MaxFiles = 10000,
    [ValidateRange(1, 1000)]
    [int]$MaxTotalGB = 2,
    [ValidateRange(1, 720)]
    [int]$RetentionHours = 24,
    [ValidateRange(1, 104857600)]
    [int]$MaxRequestBytes = 524288,
    [ValidateRange(1, 104857600)]
    [int]$MaxResponseBytes = 1048576,
    [string]$SshHost = 'x-llm',
    [string]$RemoteDirectory = '/opt/x-llm-net/data/diagnostic-captures',
    [string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'

if ($SshHost -notmatch '^[A-Za-z0-9_.@-]+$') {
    throw 'SshHost contains unsupported characters.'
}
if ($RemoteDirectory -notmatch '^/[A-Za-z0-9_./-]+$') {
    throw 'RemoteDirectory must be an absolute path containing only letters, numbers, _, ., /, or -.'
}

$remoteConfig = "$RemoteDirectory/config.json"

function Invoke-SshChecked {
    param([Parameter(Mandatory = $true)][string]$Command)
    & ssh $SshHost $Command
    if ($LASTEXITCODE -ne 0) {
        throw "ssh command failed with exit code $LASTEXITCODE"
    }
}

function Publish-Config {
    param([Parameter(Mandatory = $true)][System.Collections.IDictionary]$Config)

    $localTemporary = [System.IO.Path]::GetTempFileName()
    $remoteTemporary = "$remoteConfig.tmp-$PID"
    try {
        $json = $Config | ConvertTo-Json -Depth 8
        $utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($localTemporary, $json + [Environment]::NewLine, $utf8WithoutBom)

        Invoke-SshChecked "mkdir -p '$RemoteDirectory' && chmod 700 '$RemoteDirectory'"
        & scp $localTemporary "${SshHost}:$remoteTemporary"
        if ($LASTEXITCODE -ne 0) {
            throw "scp failed with exit code $LASTEXITCODE"
        }
        Invoke-SshChecked "chmod 600 '$remoteTemporary' && mv -f '$remoteTemporary' '$remoteConfig'"
    }
    finally {
        Remove-Item -LiteralPath $localTemporary -Force -ErrorAction SilentlyContinue
        & ssh $SshHost "rm -f '$remoteTemporary'" 2>$null | Out-Null
    }
}

switch ($Action) {
    'enable' {
        $filterCount = $Model.Count + $UpstreamModel.Count + $Channel.Count + $UserId.Count + $TokenId.Count + $Group.Count + $Protocol.Count
        if ($filterCount -eq 0) {
            throw 'At least one filter is required: Model, UpstreamModel, Channel, UserId, TokenId, Group, or Protocol.'
        }
        if ([string]::IsNullOrWhiteSpace($Name)) {
            throw 'Name cannot be empty.'
        }

        $rule = [ordered]@{
            name             = $Name.Trim()
            requested_models = @($Model)
            upstream_models  = @($UpstreamModel)
            channel_ids      = @($Channel)
            user_ids         = @($UserId)
            token_ids        = @($TokenId)
            groups           = @($Group)
            protocols        = @($Protocol)
            sample_rate      = $SampleRate
        }
        $config = [ordered]@{
            version            = 1
            enabled            = $true
            expires_at         = [DateTimeOffset]::Now.AddHours($Hours).ToString('o')
            rules              = @($rule)
            max_files          = $MaxFiles
            max_total_gb       = $MaxTotalGB
            retention_hours    = $RetentionHours
            delete_batch_size  = 1000
            max_request_bytes  = $MaxRequestBytes
            max_response_bytes = $MaxResponseBytes
        }
        Publish-Config $config
        Write-Host "Diagnostic capture '$Name' enabled for $Hours hour(s)."
        Write-Host "The running service will reload it within about 2 seconds; no restart is required."
    }
    'disable' {
        $config = [ordered]@{
            version            = 1
            enabled            = $false
            expires_at         = [DateTimeOffset]::Now.ToString('o')
            rules              = @()
            max_files          = $MaxFiles
            max_total_gb       = $MaxTotalGB
            retention_hours    = $RetentionHours
            delete_batch_size  = 1000
            max_request_bytes  = $MaxRequestBytes
            max_response_bytes = $MaxResponseBytes
        }
        Publish-Config $config
        Write-Host 'Diagnostic capture disabled. Existing capture files were retained.'
    }
    'status' {
        Invoke-SshChecked "if [ -f '$remoteConfig' ]; then cat '$remoteConfig'; else echo 'config: missing (capture disabled)'; fi; printf '\nfiles: '; find '$RemoteDirectory' -type f -name '*.capture' 2>/dev/null | wc -l; printf 'disk: '; du -sh '$RemoteDirectory' 2>/dev/null | cut -f1 || true"
    }
    'export' {
        if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
            $OutputDirectory = Join-Path ([Environment]::GetFolderPath('MyDocuments')) ("xllm-diagnostic-captures-" + (Get-Date -Format 'yyyyMMdd-HHmmss'))
        }
        New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
        & scp -r "${SshHost}:$RemoteDirectory" $OutputDirectory
        if ($LASTEXITCODE -ne 0) {
            throw "export failed with exit code $LASTEXITCODE"
        }
        Write-Host "Captures exported to $OutputDirectory"
    }
}
