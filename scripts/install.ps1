#Requires -Version 5.1
<#
.SYNOPSIS
    ppm installer — install, update, or uninstall ppm from GitHub Releases.
    ppm 安装脚本 —— 从 GitHub Releases 安装、覆盖安装（升级）或卸载 ppm。

.DESCRIPTION
    Downloads the latest release asset for the current architecture, installs it
    to %APPDATA%\ppm, and adds that directory to the user PATH. Optionally routes
    both the release lookup and the download through a GitHub proxy (gh-proxy).

    从最新 release 下载当前架构的 exe，安装到 %APPDATA%\ppm，并把该目录加入用户
    PATH。可选通过 GitHub 加速镜像（gh-proxy）查询版本和下载。

    Interactive prompts are shown by default; pass -Yes to skip them.
    默认交互式确认；加 -Yes 可跳过确认（适合脚本/CI）。

.PARAMETER Action
    auto (default) | install | update | uninstall
    auto: 未安装则安装，已安装则覆盖安装。

.PARAMETER Mirror
    Use the GitHub proxy for both the release lookup and the download.
    使用 GitHub 加速镜像查询版本并下载。

.PARAMETER MirrorUrl
    GitHub proxy prefix (default: https://gh-proxy.com/).
    加速镜像前缀，默认 https://gh-proxy.com/ 。

.PARAMETER InstallDir
    Install directory (default: %APPDATA%\ppm).
    安装目录，默认 %APPDATA%\ppm 。

.PARAMETER Yes
    Skip every confirmation prompt.
    跳过所有确认提示。

.EXAMPLE
    irm https://raw.githubusercontent.com/wmy2981/ppm/main/scripts/install.ps1 | iex

.EXAMPLE
    iwr https://raw.githubusercontent.com/wmy2981/ppm/main/scripts/install.ps1 -OutFile install.ps1
    .\install.ps1 -Action uninstall

.NOTES
    Script language follows the Windows UI language (Chinese or English).
    脚本语言自动跟随系统 UI 语言（中文或英文）。
#>
[CmdletBinding()]
param(
    [ValidateSet('auto', 'install', 'update', 'uninstall')]
    [string]$Action = 'auto',

    [switch]$Mirror,

    [string]$MirrorUrl = 'https://gh-proxy.com/',

    [string]$InstallDir = (Join-Path $env:APPDATA 'ppm'),

    [switch]$Yes
)

$script:PrevErrorActionPreference = $ErrorActionPreference
$script:PrevProgressPreference = $ProgressPreference
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
try {
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch { }

$script:Repo = 'wmy2981/ppm'
$script:MirrorBase = $MirrorUrl
if (-not $script:MirrorBase.EndsWith('/')) { $script:MirrorBase = $script:MirrorBase + '/' }
$script:UseMirror = [bool]$Mirror
$script:AssumeYes = [bool]$Yes
$script:Headers = @{
    'User-Agent' = 'ppm-installer'
    'Accept'     = 'application/vnd.github+json'
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $env:USERPROFILE 'AppData\Roaming\ppm'
}
$script:InstallDir = $InstallDir.TrimEnd('\')
$script:ExePath = Join-Path $script:InstallDir 'ppm.exe'

# ---------------------------------------------------------------- language ---
$script:Lang = 'en'
try {
    if ([System.Globalization.CultureInfo]::CurrentUICulture.TwoLetterISOLanguageName -eq 'zh') {
        $script:Lang = 'zh'
    }
} catch { }

$Strings = @{
    zh = @{
        Header            = 'ppm 安装程序'
        RepoLine          = '仓库：{0}'
        Arch              = '检测到架构：{0}'
        Latest            = '正在获取最新版本…'
        LatestOk          = '最新版本：{0}（{1}）'
        MirrorHint        = '提示：直连 GitHub 下载可能很慢，可使用加速镜像。'
        AskMirror         = '是否使用 GitHub 加速镜像（{0}）获取版本信息和下载？'
        MirrorOn          = '已启用加速镜像：{0}'
        MirrorFallback    = '加速镜像不可用（{0}），改用 GitHub 直连。'
        Downloading       = '正在下载 {0} …'
        Downloaded        = '下载完成：{0}'
        InstallTo         = '安装目录：{0}'
        ConfirmInstall    = '即将安装 ppm {0}（{1}）到上述目录，并把该目录加入用户 PATH。继续吗？'
        AlreadyInstalled  = '检测到已安装的 ppm {0}。'
        AlreadyUnknown    = '检测到已安装的 ppm（版本未知）。'
        ConfirmUpdate     = '是否覆盖安装为 {0}？'
        ConfirmUninstall  = '即将删除 {0} 并从用户 PATH 移除该目录（备注与备份会保留）。继续吗？'
        ConfirmCleanup    = '未找到 ppm.exe，但用户 PATH 仍包含该目录。是否移除该 PATH 条目？'
        NotInstalled      = '未在 {0} 找到 ppm.exe，无需卸载。'
        PathAdded         = '已加入用户 PATH：{0}'
        PathExists        = '用户 PATH 中已存在该目录，跳过。'
        PathRemoved       = '已从用户 PATH 移除：{0}'
        PathMissing       = '用户 PATH 中未找到该目录，跳过。'
        DoneInstall       = '安装完成。请打开新的终端窗口，然后运行：ppm --help'
        DoneUpdate        = '覆盖安装完成，版本：{0}'
        DoneUninstall     = '已卸载 ppm。'
        DataKept          = '备注与备份仍保留在 {0}，如需一并删除请手动操作。'
        Cancelled         = '已取消。'
        InvalidAnswer     = '请输入 y 或 n。'
        NoRelease         = '无法获取最新版本信息：{0}'
        NoAsset           = '最新 release 中没有 windows-{0} 的可执行文件。'
        DownloadFailedAll = '下载失败（镜像与直连均不可用）：{0}'
        ExeLocked         = '无法写入 {0}，ppm.exe 可能正在运行，请关闭后重试。'
        Failed            = '安装失败：{0}'
    }
    en = @{
        Header            = 'ppm installer'
        RepoLine          = 'Repository: {0}'
        Arch              = 'Detected architecture: {0}'
        Latest            = 'Looking up the latest release...'
        LatestOk          = 'Latest release: {0} ({1})'
        MirrorHint        = 'Tip: direct GitHub downloads can be slow; a mirror can be used.'
        AskMirror         = 'Use a GitHub proxy ({0}) for the release lookup and the download?'
        MirrorOn          = 'Using GitHub proxy: {0}'
        MirrorFallback    = 'Proxy unavailable ({0}); falling back to direct GitHub.'
        Downloading       = 'Downloading {0} ...'
        Downloaded        = 'Downloaded: {0}'
        InstallTo         = 'Install directory: {0}'
        ConfirmInstall    = 'Install ppm {0} ({1}) into the directory above and add it to your user PATH?'
        AlreadyInstalled  = 'Found an existing ppm {0}.'
        AlreadyUnknown    = 'Found an existing ppm (version unknown).'
        ConfirmUpdate     = 'Overwrite it with {0}?'
        ConfirmUninstall  = 'Remove {0} and drop the directory from your user PATH (notes and backups are kept)?'
        ConfirmCleanup    = 'ppm.exe was not found, but the user PATH still contains the directory. Remove that PATH entry?'
        NotInstalled      = 'ppm.exe was not found in {0}; nothing to uninstall.'
        PathAdded         = 'Added to user PATH: {0}'
        PathExists        = 'Directory already in user PATH; skipped.'
        PathRemoved       = 'Removed from user PATH: {0}'
        PathMissing       = 'Directory not in user PATH; skipped.'
        DoneInstall       = 'Installed. Open a new terminal and run: ppm --help'
        DoneUpdate        = 'Overwrite install finished, version {0}'
        DoneUninstall     = 'ppm has been uninstalled.'
        DataKept          = 'Notes and backups are still in {0}; delete them manually if you want.'
        Cancelled         = 'Cancelled.'
        InvalidAnswer     = 'Please answer y or n.'
        NoRelease         = 'Could not resolve the latest release: {0}'
        NoAsset           = 'The latest release has no windows-{0} executable.'
        DownloadFailedAll = 'Download failed (proxy and direct both unavailable): {0}'
        ExeLocked         = 'Cannot write {0}; ppm.exe may be running. Close it and retry.'
        Failed            = 'Installation failed: {0}'
    }
}
$script:Str = $Strings[$script:Lang]

function T {
    param([string]$Key, [object[]]$Value)
    $text = $script:Str[$Key]
    if ($null -eq $text) { return $Key }
    if ($null -ne $Value -and $Value.Count -gt 0) { return ($text -f $Value) }
    return $text
}

function Confirm-Step {
    param([string]$Prompt, [bool]$DefaultYes)
    if ($script:AssumeYes) { return $true }
    $hint = 'y/N'
    if ($DefaultYes) { $hint = 'Y/n' }
    while ($true) {
        $answer = Read-Host ('{0} [{1}]' -f $Prompt, $hint)
        if ($null -eq $answer -or $answer.Trim() -eq '') { return $DefaultYes }
        switch ($answer.Trim().ToLowerInvariant()) {
            'y' { return $true }
            'yes' { return $true }
            'n' { return $false }
            'no' { return $false }
        }
        Write-Host (T 'InvalidAnswer')
    }
}

function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($env:PROCESSOR_ARCHITEW6432) { $arch = $env:PROCESSOR_ARCHITEW6432 }
    if ([string]::IsNullOrWhiteSpace($arch)) { $arch = 'AMD64' }
    switch ($arch.ToUpperInvariant()) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        'X86' { return '386' }
    }
    return 'amd64'
}

function Get-LatestRelease {
    param([string]$Arch)
    $api = 'https://api.github.com/repos/{0}/releases/latest' -f $script:Repo
    $urls = @()
    if ($script:UseMirror) { $urls += ($script:MirrorBase + $api) }
    $urls += $api

    $release = $null
    $lastError = $null
    for ($i = 0; $i -lt $urls.Count; $i++) {
        try {
            $release = Invoke-RestMethod -Uri $urls[$i] -Headers $script:Headers `
                -UseBasicParsing -TimeoutSec 30
            break
        } catch {
            $lastError = $_.Exception.Message
            if ($script:UseMirror -and $i -lt ($urls.Count - 1)) {
                Write-Warning ((T 'MirrorFallback') -f $lastError)
            }
        }
    }
    if ($null -eq $release) { throw ((T 'NoRelease') -f $lastError) }

    $asset = $release.assets | Where-Object { $_.name -like ('*-windows-{0}.exe' -f $Arch) } |
        Select-Object -First 1
    if ($null -eq $asset) { throw ((T 'NoAsset') -f $Arch) }

    return [pscustomobject]@{
        Tag  = $release.tag_name
        Name = $asset.name
        Url  = $asset.browser_download_url
    }
}

function Save-Asset {
    param([string]$Url, [string]$Dest)
    $urls = @()
    if ($script:UseMirror) { $urls += ($script:MirrorBase + $Url) }
    $urls += $Url

    $lastError = $null
    for ($i = 0; $i -lt $urls.Count; $i++) {
        try {
            Invoke-WebRequest -Uri $urls[$i] -OutFile $Dest -UseBasicParsing -TimeoutSec 600
            return
        } catch {
            $lastError = $_.Exception.Message
            if (Test-Path -LiteralPath $Dest) {
                Remove-Item -LiteralPath $Dest -Force -ErrorAction SilentlyContinue
            }
            if ($script:UseMirror -and $i -lt ($urls.Count - 1)) {
                Write-Warning ((T 'MirrorFallback') -f $lastError)
            }
        }
    }
    throw ((T 'DownloadFailedAll') -f $lastError)
}

function Get-InstalledVersion {
    param([string]$Exe)
    if (-not (Test-Path -LiteralPath $Exe)) { return $null }
    try {
        $out = & $Exe version 2>$null | Out-String
        $match = [regex]::Match($out, '(\d+\.\d+\.\d+)')
        if ($match.Success) { return $match.Value }
    } catch { }
    return $null
}

function Test-SameDir {
    param([string]$Entry, [string]$Dir)
    return ($Entry.TrimEnd('\').ToLowerInvariant() -eq $Dir.TrimEnd('\').ToLowerInvariant())
}

function Test-InUserPath {
    param([string]$Dir)
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($current)) { return $false }
    foreach ($item in @($current -split ';' | Where-Object { $_ -ne '' })) {
        if (Test-SameDir -Entry $item -Dir $Dir) { return $true }
    }
    return $false
}

function Add-ToUserPath {
    param([string]$Dir)
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($null -eq $current) { $current = '' }

    $items = @($current -split ';' | Where-Object { $_ -ne '' })
    foreach ($item in $items) {
        if (Test-SameDir -Entry $item -Dir $Dir) {
            Write-Host (T 'PathExists')
            return $false
        }
    }

    $newPath = $Dir
    if ($current.Trim() -ne '') { $newPath = $current.TrimEnd(';') + ';' + $Dir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    Write-Host ((T 'PathAdded') -f $Dir)
    return $true
}

function Remove-FromUserPath {
    param([string]$Dir)
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($current)) {
        Write-Host (T 'PathMissing')
        return
    }

    $items = @($current -split ';' | Where-Object { $_ -ne '' })
    $kept = @($items | Where-Object { -not (Test-SameDir -Entry $_ -Dir $Dir) })
    if ($kept.Count -eq $items.Count) {
        Write-Host (T 'PathMissing')
        return
    }

    [Environment]::SetEnvironmentVariable('Path', ($kept -join ';'), 'User')
    Write-Host ((T 'PathRemoved') -f $Dir)
}

function Install-Ppm {
    param([string]$Arch)

    Write-Host (T 'Latest')
    $release = Get-LatestRelease -Arch $Arch
    Write-Host ((T 'LatestOk') -f $release.Tag, $release.Name)

    $isUpdate = $false
    if (Test-Path -LiteralPath $script:ExePath) {
        $isUpdate = $true
        $installed = Get-InstalledVersion -Exe $script:ExePath
        if ($null -ne $installed) {
            Write-Host ((T 'AlreadyInstalled') -f $installed)
        } else {
            Write-Host (T 'AlreadyUnknown')
        }
        if (-not (Confirm-Step -Prompt ((T 'ConfirmUpdate') -f $release.Tag) -DefaultYes $true)) {
            Write-Host (T 'Cancelled')
            return
        }
    } else {
        Write-Host ((T 'InstallTo') -f $script:InstallDir)
        if (-not (Confirm-Step -Prompt ((T 'ConfirmInstall') -f $release.Tag, $Arch) -DefaultYes $true)) {
            Write-Host (T 'Cancelled')
            return
        }
    }

    if (-not (Test-Path -LiteralPath $script:InstallDir)) {
        New-Item -ItemType Directory -Path $script:InstallDir -Force | Out-Null
    }

    $temp = Join-Path $script:InstallDir 'ppm.exe.new'
    Write-Host ((T 'Downloading') -f $release.Name)
    Save-Asset -Url $release.Url -Dest $temp

    try {
        Move-Item -LiteralPath $temp -Destination $script:ExePath -Force
    } catch {
        Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue
        throw ((T 'ExeLocked') -f $script:ExePath)
    }

    $sizeMb = [math]::Round((Get-Item -LiteralPath $script:ExePath).Length / 1MB, 1)
    Write-Host ((T 'Downloaded') -f ('{0} MB' -f $sizeMb))

    Add-ToUserPath -Dir $script:InstallDir | Out-Null
    if ($env:Path -notlike ('*' + $script:InstallDir + '*')) {
        $env:Path = $env:Path + ';' + $script:InstallDir
    }

    Write-Host ''
    if ($isUpdate) {
        Write-Host ((T 'DoneUpdate') -f $release.Tag) -ForegroundColor Green
    } else {
        Write-Host (T 'DoneInstall') -ForegroundColor Green
    }
}

function Uninstall-Ppm {
    $exeExists = Test-Path -LiteralPath $script:ExePath
    $inPath = Test-InUserPath -Dir $script:InstallDir

    if (-not $exeExists -and -not $inPath) {
        Write-Host ((T 'NotInstalled') -f $script:InstallDir)
        return
    }

    if ($exeExists) {
        if (-not (Confirm-Step -Prompt ((T 'ConfirmUninstall') -f $script:ExePath) -DefaultYes $false)) {
            Write-Host (T 'Cancelled')
            return
        }
        try {
            Remove-Item -LiteralPath $script:ExePath -Force
        } catch {
            throw ((T 'ExeLocked') -f $script:ExePath)
        }
    } else {
        if (-not (Confirm-Step -Prompt (T 'ConfirmCleanup') -DefaultYes $false)) {
            Write-Host (T 'Cancelled')
            return
        }
    }

    Remove-FromUserPath -Dir $script:InstallDir

    Write-Host ''
    Write-Host (T 'DoneUninstall') -ForegroundColor Green
    if (Test-Path -LiteralPath $script:InstallDir) {
        Write-Host ((T 'DataKept') -f $script:InstallDir)
    }
}

# -------------------------------------------------------------------- main ---
try {
    Write-Host ''
    Write-Host ('== {0} ==' -f (T 'Header')) -ForegroundColor Cyan
    Write-Host ((T 'RepoLine') -f $script:Repo)

    $arch = Get-Arch
    Write-Host ((T 'Arch') -f $arch)

    $effectiveAction = $Action
    if ($effectiveAction -eq 'auto') {
        if (Test-Path -LiteralPath $script:ExePath) {
            $effectiveAction = 'update'
        } else {
            $effectiveAction = 'install'
        }
    }

    if ($effectiveAction -eq 'uninstall') {
        Uninstall-Ppm
        return
    }

    if (-not $PSBoundParameters.ContainsKey('Mirror')) {
        if ($script:AssumeYes) {
            $script:UseMirror = $false
        } else {
            Write-Host (T 'MirrorHint')
            $script:UseMirror = Confirm-Step -Prompt ((T 'AskMirror') -f $script:MirrorBase) -DefaultYes $false
        }
    }
    if ($script:UseMirror) {
        Write-Host ((T 'MirrorOn') -f $script:MirrorBase)
    }

    Install-Ppm -Arch $arch
} catch {
    Write-Host ''
    Write-Host ((T 'Failed') -f $_.Exception.Message) -ForegroundColor Red
} finally {
    $ErrorActionPreference = $script:PrevErrorActionPreference
    $ProgressPreference = $script:PrevProgressPreference
}
