; VideoDelite NSIS installer (Phase 11, plan §111)
; Build:  makensis /DVERSION=1.0.0 /DSTAGE=build\dist\app installer\installer.nsi
;
; Behaviors (frozen):
;   - installs to Program Files\VideoDelite
;   - upgrade: silently removes the previous installation first
;   - uninstall: user data in %LOCALAPPDATA%\VideoDelite is PRESERVED by
;     default; an explicit checkbox offers full removal (plan §24/§25)

Unicode true
ManifestDPIAware true

!include "MUI2.nsh"
!include "x64.nsh"

!define PRODUCT      "VideoDelite"
!define EXE          "VideoDelite.exe"
!define REGKEY       "Software\Microsoft\Windows\CurrentVersion\Uninstall\VideoDelite"
!define LOCALDATA    "$LOCALAPPDATA\VideoDelite"

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef STAGE
  !define STAGE "build\dist\app"
!endif

Name "${PRODUCT} ${VERSION}"
OutFile "..\build\dist\${PRODUCT}-${VERSION}-setup.exe"
InstallDir "$PROGRAMFILES64\${PRODUCT}"
InstallDirRegKey HKLM "${REGKEY}" "InstallLocation"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!define MUI_ICON "..\build\windows\icon.ico"
!define MUI_UNICON "..\build\windows\icon.ico"
!define MUI_ABORTWARNING

; Language order: zh-CN first (first-run product language follows Windows)
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
UninstPage custom un.PageUserData un.PageUserDataLeave
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; ---------- install section ----------

Section "Install"
  SetOutPath "$INSTDIR"

  ; upgrade path: run the old uninstaller silently if present
  ReadRegStr $R0 HKLM "${REGKEY}" "UninstallString"
  ${If} $R0 != ""
    DetailPrint "Removing previous version..."
    ExecWait '"$R0" /S _?=$INSTDIR'
  ${EndIf}

  ; application files (from STAGE)
  File "/oname=${EXE}" "${STAGE}\${EXE}"
  File "/oname=THIRD-PARTY-NOTICES.txt" "${STAGE}\THIRD-PARTY-NOTICES.txt"

  ; ffmpeg/ffprobe distribution
  SetOutPath "$INSTDIR\bin"
  File "${STAGE}\bin\ffmpeg.exe"
  File "${STAGE}\bin\ffprobe.exe"
  ; shared DLLs shipped with the ffmpeg build
  File /nonfatal "${STAGE}\bin\*.dll"

  ; start menu shortcuts
  CreateDirectory "$SMPROGRAMS\${PRODUCT}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT}\${PRODUCT}.lnk" "$INSTDIR\${EXE}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT}\卸载 ${PRODUCT}.lnk" "$INSTDIR\Uninstall.exe"

  ; optional desktop shortcut
  MessageBox MB_YESNO|MB_ICONQUESTION "是否创建桌面快捷方式？" /SD IDYES IDNO +2
  CreateShortcut "$DESKTOP\${PRODUCT}.lnk" "$INSTDIR\${EXE}"

  ; registry (uninstall entry)
  WriteRegStr HKLM "${REGKEY}" "DisplayName" "${PRODUCT}"
  WriteRegStr HKLM "${REGKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${REGKEY}" "Publisher" "VideoDelite Project"
  WriteRegStr HKLM "${REGKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${REGKEY}" "DisplayIcon" "$INSTDIR\${EXE}"
  WriteRegStr HKLM "${REGKEY}" "UninstallString" "$INSTDIR\Uninstall.exe"
  WriteRegDWORD HKLM "${REGKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${REGKEY}" "NoRepair" 1
  WriteUninstaller "$INSTDIR\Uninstall.exe"

  ; Data lives inside the install dir (user request): grant the built-in
  ; Users group modify rights so the app (runs as normal user) can write
  ; its Data subfolder under Program Files. SID S-1-5-32-545 = Users,
  ; locale-independent.
  CreateDirectory "$INSTDIR\Data"
  nsExec::Exec 'icacls "$INSTDIR\Data" /grant *S-1-5-32-545:(OI)(CI)M'
SectionEnd

Function .onInit
  ${IfNot} ${IsNativeAMD64}
    MessageBox MB_ICONSTOP "VideoDelite 仅支持 64 位 Windows。"
    Abort
  ${EndIf}
FunctionEnd

; ---------- uninstaller ----------

Var UserDataChoice
Var UserDataCheckbox

Function un.PageUserData
  !insertmacro MUI_HEADER_TEXT "删除用户数据" "选择卸载时是否同时删除本地用户数据"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 24u "您的视频文件不受影响。以下选项只影响 %LOCALAPPDATA%\VideoDelite（历史记录、设置、日志）。"
  Pop $0
  ${NSD_CreateCheckbox} 0 40u 100% 12u "同时删除用户数据（历史记录 / 设置 / 日志 / 缓存）"
  Pop $UserDataCheckbox
  ${NSD_SetState} $UserDataCheckbox ${BST_UNCHECKED}
  nsDialogs::Show
FunctionEnd

Function un.PageUserDataLeave
  ${NSD_GetState} $UserDataCheckbox $UserDataChoice
FunctionEnd

Section "Uninstall"
  ; remove program files (installed set)
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
  Delete "$INSTDIR\bin\ffmpeg.exe"
  Delete "$INSTDIR\bin\ffprobe.exe"
  Delete "$INSTDIR\bin\*.dll"
  RMDir "$INSTDIR\bin"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  ; shortcuts
  Delete "$SMPROGRAMS\${PRODUCT}\${PRODUCT}.lnk"
  Delete "$SMPROGRAMS\${PRODUCT}\卸载 ${PRODUCT}.lnk"
  RMDir "$SMPROGRAMS\${PRODUCT}"
  Delete "$DESKTOP\${PRODUCT}.lnk"

  DeleteRegKey HKLM "${REGKEY}"

  ; optional user data removal (explicit opt-in only)
  ${If} $UserDataChoice == ${BST_CHECKED}
    DetailPrint "Removing user data..."
    RMDir /r "${LOCALDATA}"
  ${EndIf}
SectionEnd
