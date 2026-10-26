[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$composeFiles = @(
    "-f", (Join-Path $repoRoot "docker-compose.yml"),
    "-f", (Join-Path $repoRoot "docker-compose.demo.yml")
)
$tunnelContainer = "wm-demo-tunnel"
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

function Remove-TunnelContainer {
    for ($attempt = 1; $attempt -le 5; $attempt++) {
        $existingTunnel = docker ps -a --filter "name=^/$tunnelContainer`$" --format "{{.Names}}"
        Assert-LastExitCode "No se pudo comprobar la limpieza del túnel"
        if ($existingTunnel -notcontains $tunnelContainer) {
            return
        }

        $previousPreference = $ErrorActionPreference
        try {
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

try {
    Set-Location $repoRoot

    docker info *> $null
    Assert-LastExitCode "Docker no está disponible"

    Remove-TunnelContainer

    docker compose @composeFiles stop demo-gateway
    Assert-LastExitCode "No se pudo detener el gateway de demo"

    Remove-Item Env:\DEMO_PUBLIC_URL -ErrorAction SilentlyContinue
    Remove-Item Env:\DEMO_GATEWAY_PORT -ErrorAction SilentlyContinue
    docker compose -f (Join-Path $repoRoot "docker-compose.yml") up -d --no-deps --force-recreate backend
    Assert-LastExitCode "No se pudo restaurar la configuración normal del backend"
    Wait-ForHealthyBackend

    # El modo demo hornea `same-origin` en el bundle de Next.js. Al detenerlo,
    # reconstruimos la imagen normal para que localhost vuelva a hablar directo
    # con PUBLIC_API_URL/PUBLIC_WS_URL desde .env.
    docker compose -f (Join-Path $repoRoot "docker-compose.yml") build web
    Assert-LastExitCode "No se pudo reconstruir el frontend local"
    docker compose -f (Join-Path $repoRoot "docker-compose.yml") up -d --no-deps --force-recreate web
    Assert-LastExitCode "No se pudo restaurar el frontend local"

    Write-Host "Demo detenida. Backend y frontend volvieron a usar la configuración de .env." -ForegroundColor Green
}
finally {
    Set-Location $previousLocation
}
