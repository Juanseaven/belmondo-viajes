[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$GatewayPort = 8088
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$composeFiles = @(
    "-f", (Join-Path $repoRoot "docker-compose.yml"),
    "-f", (Join-Path $repoRoot "docker-compose.demo.yml")
)
$tunnelContainer = "wm-demo-tunnel"
$tunnelStarted = $false
$previousLocation = Get-Location

function Assert-LastExitCode([string]$Action) {
    if ($LASTEXITCODE -ne 0) {
        throw "$Action (exit $LASTEXITCODE)"
    }
}

function Wait-ForHealthyBackend {
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        $status = docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' wm-backend 2>$null
        if ($LASTEXITCODE -eq 0 -and $status.Trim() -eq "healthy") {
            return
        }
        Start-Sleep -Seconds 1
    }
    throw "El backend no llegó a estado healthy dentro de 60 segundos."
}

function Wait-ForHttpOk([string]$Url, [string]$Description, [int]$Attempts = 60) {
    $lastError = $null
    for ($attempt = 1; $attempt -le $Attempts; $attempt++) {
        try {
            $response = Invoke-WebRequest $Url -UseBasicParsing -TimeoutSec 10
            if ($response.StatusCode -eq 200) {
                return $response
            }
            $lastError = "HTTP $($response.StatusCode)"
        }
        catch {
            $lastError = $_.Exception.Message
        }
        Start-Sleep -Seconds 1
    }
    throw "$Description no respondió HTTP 200 después de $Attempts intentos. Último error: $lastError"
}

function Remove-TunnelContainer {
    for ($attempt = 1; $attempt -le 5; $attempt++) {
        $existingTunnel = docker ps -a --filter "name=^/$tunnelContainer`$" --format "{{.Names}}"
        Assert-LastExitCode "No se pudo comprobar la limpieza del túnel"
        if ($existingTunnel -notcontains $tunnelContainer) {
            return
        }

        $previousPreference = $ErrorActionPreference
        try {
            # Docker puede devolver un error transitorio si el proceso acaba de
            # finalizar. Reintentamos y confirmamos la ausencia por separado.
            $ErrorActionPreference = "Continue"
            docker rm -f $tunnelContainer *> $null
        }
        finally {
            $ErrorActionPreference = $previousPreference
        }

        Start-Sleep -Seconds 1
    }

    throw "No se pudo confirmar la eliminación de $tunnelContainer después de cinco intentos."
}

function Assert-PublicBackendReachable([string]$BaseUrl) {
    $lastError = $null
    for ($attempt = 1; $attempt -le 30; $attempt++) {
        try {
            $response = Invoke-WebRequest "$BaseUrl/api/me" -UseBasicParsing -TimeoutSec 10
            $lastError = "HTTP $($response.StatusCode); se esperaba 401 sin autenticación"
        }
        catch {
            $statusCode = $null
            if ($_.Exception.Response) {
                $statusCode = [int]$_.Exception.Response.StatusCode
            }
            if ($statusCode -eq 401) {
                return
            }
            $lastError = $_.Exception.Message
        }
        Start-Sleep -Seconds 1
    }
    throw "La API pública no quedó accesible a través del gateway. Último error: $lastError"
}

try {
    Set-Location $repoRoot

    docker info *> $null
    Assert-LastExitCode "Docker no está disponible"

    $existingTunnel = docker ps -a --filter "name=^/$tunnelContainer`$" --format "{{.Names}}"
    Assert-LastExitCode "No se pudo consultar el estado del túnel"
    if ($existingTunnel -contains $tunnelContainer) {
        throw "Ya existe $tunnelContainer. Ejecutá scripts/stop-demo.ps1 antes de iniciar otra demo."
    }

    $env:DEMO_GATEWAY_PORT = $GatewayPort.ToString()
    # Sólo el frontend necesita un build especial (same-origin). El backend se
    # recrea más abajo con la URL efímera, usando su imagen existente.
    docker compose @composeFiles build web
    Assert-LastExitCode "No se pudo construir el frontend de demo"
    docker compose @composeFiles up -d demo-gateway
    Assert-LastExitCode "No se pudo iniciar el gateway de demo"

    Wait-ForHttpOk "http://127.0.0.1:$GatewayPort/login" "El gateway local" | Out-Null

    $publicUrl = $null
    $lastTunnelLogs = ""
    for ($tunnelAttempt = 1; $tunnelAttempt -le 3 -and -not $publicUrl; $tunnelAttempt++) {
        docker run -d --name $tunnelContainer `
            --network whatsapp-mirror_backbone `
            cloudflare/cloudflared:latest `
            tunnel --no-autoupdate --url http://demo-gateway:8080 | Out-Null
        Assert-LastExitCode "No se pudo iniciar cloudflared"
        $tunnelStarted = $true

        for ($attempt = 0; $attempt -lt 60; $attempt++) {
            # Windows PowerShell convierte stderr redirigido de un proceso nativo
            # en ErrorRecord. cloudflared escribe sus logs normales por stderr.
            $previousErrorActionPreference = $ErrorActionPreference
            $ErrorActionPreference = "Continue"
            $logs = (docker logs $tunnelContainer 2>&1 | Out-String)
            $ErrorActionPreference = $previousErrorActionPreference
            if ($logs -match 'https://(?!api\.)[a-z0-9-]+\.trycloudflare\.com') {
                $publicUrl = $Matches[0]
                break
            }

            $running = docker inspect --format '{{.State.Running}}' $tunnelContainer 2>$null
            if ($LASTEXITCODE -ne 0 -or $running.Trim() -ne "true") {
                break
            }
            Start-Sleep -Seconds 1
        }

        if (-not $publicUrl) {
            $lastTunnelLogs = $logs
            Remove-TunnelContainer
            $tunnelStarted = $false
            if ($tunnelAttempt -lt 3) {
                Start-Sleep -Seconds 2
            }
        }
    }

    if (-not $publicUrl) {
        throw "Cloudflare no entregó una URL después de tres intentos.`n$lastTunnelLogs"
    }

    $env:DEMO_PUBLIC_URL = $publicUrl
    docker compose @composeFiles up -d --no-deps --force-recreate backend
    Assert-LastExitCode "No se pudo configurar el backend con la URL pública"
    Wait-ForHealthyBackend

    # Nginx resuelve el nombre del backend al iniciar. Reiniciarlo después de
    # recrear ese contenedor evita conservar una IP anterior en el upstream.
    docker compose @composeFiles restart demo-gateway
    Assert-LastExitCode "No se pudo actualizar el gateway después de recrear el backend"

    Assert-PublicBackendReachable $publicUrl

    Wait-ForHttpOk "$publicUrl/login" "La URL pública" 30 | Out-Null

    Write-Host ""
    Write-Host "Demo disponible en: $publicUrl" -ForegroundColor Green
    Write-Host "La URL funciona mientras Docker y $tunnelContainer sigan activos."
    Write-Host "Para cerrarla: powershell -ExecutionPolicy Bypass -File scripts/stop-demo.ps1"
}
catch {
    $originalError = $_
    if ($tunnelStarted) {
        try {
            Remove-TunnelContainer
            $tunnelStarted = $false
        }
        catch {
            throw "$($originalError.Exception.Message)`nAdemás, falló la limpieza del túnel: $($_.Exception.Message)"
        }
    }
    throw $originalError
}
finally {
    Set-Location $previousLocation
}
