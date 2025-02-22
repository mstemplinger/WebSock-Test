# Test



$URL = "help.ondeso.com/HelpForwarding/wim/winre.wim"
$dest= "c:\winre\winre.wim"

if (-not( Test-Path -Path "c:\winre"))

    {

    New-Item -Path "C:\winre" -ItemType Directory
    Invoke-WebRequest -Uri $URL -OutFile $dest

    }
else {

    Invoke-WebRequest -Uri $URL -OutFile $dest

    }

if (Test-Path -Path c:\winre\winre.wim)
{

Start-Process -FilePath "reagentc.exe" -ArgumentList  "/disable" -NoNewWindow -Wait 
Start-Process -FilePath "reagentc.exe" -ArgumentList "/setreimage /path C:\winre\winre.wim" -NoNewWindow -Wait
Start-Process -FilePath "reagentc.exe" -ArgumentList "/enable" -NoNewWindow -Wait
Start-Process -FilePath "reagentc.exe" -ArgumentList "/boottore" -NoNewWindow -Wait
Restart-Computer
}

else {

Write-Host "Download error"

}
