windres main.rc -O coff -o rsrc.syso

go build -ldflags "-s -w" -o ondeso_websock.exe

rem go build -ldflags "-s -w -manifest main.manifest" -o websock.exe

