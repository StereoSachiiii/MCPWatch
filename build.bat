@echo off
setlocal enabledelayedexpansion

echo Searching for Visual Studio...

set "VSWHERE=%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe"
if not exist "!VSWHERE!" (
    echo vswhere.exe not found. Please ensure Visual Studio is installed.
    exit /b 1
)

for /f "usebackq tokens=*" %%i in (`"!VSWHERE!" -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath`) do (
  set "VS_PATH=%%i"
)

if not defined VS_PATH (
    echo Visual Studio C++ tools not found.
    exit /b 1
)

echo Found VS at: !VS_PATH!
echo Setting up environment...

call "!VS_PATH!\VC\Auxiliary\Build\vcvarsall.bat" x64

echo Compiling MCPWatch...
cd cpp
cl /EHsc /O2 /MT main.cpp sqlite3.c /link /out:../mcpwatch.exe
cd ..

if %ERRORLEVEL% EQU 0 (
    echo.
    echo [SUCCESS] mcpwatch.exe created successfully!
) else (
    echo.
    echo [ERROR] Compilation failed.
)
