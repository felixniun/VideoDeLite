# Code signing for VideoDelite (test self-signed cert).
# Production: replace with a real Authenticode cert (OV/EV) and signtool
# with RFC3161 timestamp - see plan section 79.
#
# Usage: powershell -File build\sign.ps1 [-PfxPath build\keys\test-signing.pfx] [-Files a.exe,b.exe]
param(
    [string]$PfxPath = "build\keys\test-signing.pfx",
    [string]$PfxPassword = "videodelite-test",
    [string]$Files = ""
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $PfxPath)) {
    # Create a self-signed code-signing cert once and export it.
    $cert = New-SelfSignedCertificate -Type CodeSigningCert `
        -Subject "CN=VideoDelite Dev, O=VideoDelite Project" `
        -CertStoreLocation "Cert:\CurrentUser\My" `
        -NotAfter (Get-Date).AddYears(3)
    $sec = ConvertTo-SecureString -String $PfxPassword -Force -AsPlainText
    Export-PfxCertificate -Cert $cert -FilePath $PfxPath -Password $sec | Out-Null
    Write-Host "created test cert $($cert.Thumbprint)"
}

$sec = ConvertTo-SecureString -String $PfxPassword -Force -AsPlainText
# Load the private key only for this process. Importing into CurrentUser\My
# fails in restricted environments and needlessly changes the user's store.
$flags = [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::EphemeralKeySet
$signingCert = [System.Security.Cryptography.X509Certificates.X509Certificate2]::new(
    (Resolve-Path $PfxPath).Path, $PfxPassword, $flags)
if (-not $signingCert.HasPrivateKey) { throw "signing certificate has no private key" }

$targets = @()
if ($Files -ne "") { $targets = $Files.Split(",") } else {
    $targets = @("build\bin\VideoDelite.exe", "build\dist\VideoDelite-1.0.0-setup.exe")
}
$signed = 0
foreach ($f in $targets) {
    if (-not (Test-Path $f)) { Write-Host "skip missing $f"; continue }
    $status = Set-AuthenticodeSignature -FilePath $f -Certificate $signingCert `
        -HashAlgorithm SHA256 -IncludeChain All
    if ($status.Status -in @("Valid", "UnknownError") -and $status.SignerCertificate) {
        # UnknownError with SignerCertificate = untrusted self-signed chain (expected for test cert)
        Write-Host "SIGNED $f  ($($status.Status), thumb $($status.SignerCertificate.Thumbprint))"
        $signed++
    } else {
        throw "FAILED $f status=$($status.Status) msg=$($status.StatusMessage)"
    }
}
$signingCert.Dispose()
Write-Host "signed $signed file(s)"
