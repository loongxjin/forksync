; ForkSync Windows NSIS Installer
; Build: makensis -DVERSION=0.6.0 build/windows/installer.nsi
; Output: build/bin/ForkSync-Setup-0.6.0.exe
;
; This script must be run from the repository root directory.
; All paths are relative to repo root.

!define APP_NAME "ForkSync"
!define APP_PUBLISHER "ForkSync Team"
!define APP_URL "https://github.com/loongxjin/forksync"
!define APP_EXE "ForkSync.exe"
!define APP_ID "com.forksync.app"

Name "${APP_NAME}"
OutFile "build\bin\ForkSync-Setup-${VERSION}.exe"
Unicode True
InstallDir "$LOCALAPPDATA\${APP_NAME}"
RequestExecutionLevel user
ShowInstDetails show

SetCompressor /SOLID lzma

# --- Pages ---

Page directory
Page instfiles

UninstPage uninstConfirm
UninstPage instfiles

# --- Install Section ---

Section "Install"
  SetOutPath "$INSTDIR"

  ; Copy application files (path relative to repo root)
  File "build\bin\${APP_EXE}"

  ; Start Menu shortcut
  CreateShortcut "$SMPROGRAMS\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"

  ; Desktop shortcut
  CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"

  ; Uninstaller
  WriteUninstaller "$INSTDIR\Uninstall.exe"

  ; Registry entry for Add/Remove Programs
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" \
    "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" \
    "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" \
    "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" \
    "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" \
    "UninstallString" "$\"$INSTDIR\Uninstall.exe$\""
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" \
    "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" \
    "URLInfoAbout" "${APP_URL}"

  DetailPrint "${APP_NAME} v${VERSION} installed successfully."
SectionEnd

# --- Uninstall Section ---

Section "Uninstall"
  ; Remove application files
  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  ; Remove shortcuts
  Delete "$SMPROGRAMS\${APP_NAME}.lnk"
  Delete "$DESKTOP\${APP_NAME}.lnk"

  ; Remove registry entries
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}"

  DetailPrint "${APP_NAME} uninstalled."
SectionEnd
