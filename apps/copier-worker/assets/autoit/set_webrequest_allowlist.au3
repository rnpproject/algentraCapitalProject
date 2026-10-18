; Configure MT5's WebRequest allow-list for one running terminal process.
; Args: terminal PID, URL, log file.
Opt("WinTitleMatchMode", 2)
Opt("SendKeyDelay", 20)
Opt("SendKeyDownDelay", 5)
Opt("MouseCoordMode", 2)

Global $gLog = -1
Global $gMainWindow = 0

Func LogLine($text)
   If $gLog <> -1 Then FileWrite($gLog, $text & @CRLF)
EndFunc

Func FindMainWindow($pid)
   Local $windows = WinList()
   For $i = 1 To $windows[0][0]
      Local $handle = $windows[$i][1]
      If $windows[$i][0] = "" Then ContinueLoop
      If BitAND(WinGetState($handle), 2) = 0 Then ContinueLoop
      If WinGetProcess($handle) = $pid Then Return $handle
   Next
   Return 0
EndFunc

Func FindOptionsWindow($pid, $timeoutMs)
   Local $timer = TimerInit()
   While TimerDiff($timer) < $timeoutMs
      Local $windows = WinList("Options")
      For $i = 1 To $windows[0][0]
         Local $handle = $windows[$i][1]
         If WinGetProcess($handle) = $pid Then Return $handle
      Next
      ; Non-English MT5 builds may translate the dialog title. Fall back to
      ; another visible top-level window owned by this exact terminal process.
      Local $owned = WinList()
      For $i = 1 To $owned[0][0]
         Local $ownedHandle = $owned[$i][1]
         If $ownedHandle = 0 Or $ownedHandle = $gMainWindow Then ContinueLoop
         If $owned[$i][0] = "" Or BitAND(WinGetState($ownedHandle), 2) = 0 Then ContinueLoop
         If WinGetProcess($ownedHandle) = $pid Then Return $ownedHandle
      Next
      Sleep(200)
   WEnd
   Return 0
EndFunc

Func FindWebRequestCheckbox($handle)
   For $i = 1 To 30
      Local $control = "Button" & $i
      Local $position = ControlGetPos($handle, "", $control)
      If @error Then ContinueLoop
      If ControlCommand($handle, "", $control, "IsVisible", "") <> 1 Then ContinueLoop
      If StringInStr(ControlGetText($handle, "", $control), "WebRequest") > 0 Then Return $control
   Next
   Return ""
EndFunc

Func ListContainsUrl($handle, $url)
   Local $count = ControlListView($handle, "", "SysListView321", "GetItemCount")
   For $i = 0 To $count - 1
      If StringLower(StringStripWS(ControlListView($handle, "", "SysListView321", "GetText", $i, 0), 3)) = StringLower($url) Then Return True
   Next
   Return False
EndFunc

Func FindConfirmButton($handle)
   ; Match the complete caption. A substring search for "ok" can match
   ; unrelated captions such as "Broker" and click a different control.
   Local $accepted[10] = ["ok", "okay", "&ok", "aceptar", "taip", "oui", "ja", "확인", "确定", "ОК"]
   For $i = 1 To 25
      Local $control = "Button" & $i
      Local $position = ControlGetPos($handle, "", $control)
      If @error Then ContinueLoop
      If ControlCommand($handle, "", $control, "IsVisible", "") <> 1 Then ContinueLoop
      Local $caption = StringLower(StringStripWS(ControlGetText($handle, "", $control), 3))
      For $j = 0 To UBound($accepted) - 1
         If $caption = $accepted[$j] Then Return $control
      Next
   Next
   Return ""
EndFunc

Func WaitOptionsClosed($handle, $timeoutMs)
   Local $timer = TimerInit()
   While TimerDiff($timer) < $timeoutMs
      If Not WinExists($handle) Then Return True
      Sleep(100)
   WEnd
   Return Not WinExists($handle)
EndFunc

If $CmdLine[0] < 3 Then Exit 10
Local $pid = Int($CmdLine[1])
Local $url = StringStripWS($CmdLine[2], 3)
Local $logPath = $CmdLine[3]
$gLog = FileOpen($logPath, 2)
If $gLog = -1 Then Exit 11
LogLine("begin pid=" & $pid & " url=" & $url)

Local $main = FindMainWindow($pid)
If $main = 0 Then
   LogLine("RESULT=FAIL reason=terminal_window_not_found")
   FileClose($gLog)
   Exit 2
EndIf
$gMainWindow = $main
WinActivate($main)
If WinWaitActive($main, "", 5) = 0 Then
   LogLine("RESULT=FAIL reason=terminal_window_not_active")
   FileClose($gLog)
   Exit 3
EndIf
Send("^o")
Local $options = FindOptionsWindow($pid, 10000)
If $options = 0 Then
   LogLine("RESULT=FAIL reason=options_dialog_not_found")
   FileClose($gLog)
   Exit 4
EndIf
WinActivate($options)
If WinWaitActive($options, "", 5) = 0 Then
   LogLine("RESULT=FAIL reason=options_dialog_not_active")
   Send("{ESC}")
   FileClose($gLog)
   Exit 4
EndIf
Sleep(350)

Local $checkbox = FindWebRequestCheckbox($options)
Local $tabTry = 0
While $checkbox = "" And $tabTry < 12
   Send("^{TAB}")
   Sleep(250)
   $checkbox = FindWebRequestCheckbox($options)
   $tabTry += 1
WEnd
If $checkbox = "" Then
   LogLine("RESULT=FAIL reason=webrequest_checkbox_not_found")
   Send("{ESC}")
   FileClose($gLog)
   Exit 5
EndIf
If ControlCommand($options, "", $checkbox, "IsChecked", "") <> 1 Then
   ControlClick($options, "", $checkbox)
   Sleep(300)
EndIf

Local $listPos = ControlGetPos($options, "", "SysListView321")
If @error Or Not IsArray($listPos) Then
   LogLine("RESULT=FAIL reason=url_list_not_found")
   Send("{ESC}")
   FileClose($gLog)
   Exit 6
EndIf
If Not ListContainsUrl($options, $url) Then
   Local $count = ControlListView($options, "", "SysListView321", "GetItemCount")
   If $count < 1 Then
      LogLine("RESULT=FAIL reason=url_list_empty")
      Send("{ESC}")
      FileClose($gLog)
      Exit 7
   EndIf
   Local $rowY = 10 + (($count - 1) * 17)
   If $rowY > $listPos[3] - 4 Then $rowY = $listPos[3] - 8
   ControlClick($options, "", "SysListView321", "left", 2, Int($listPos[2] / 2), $rowY)
   Sleep(250)
   Send("^a")
   Send($url, 1)
   Sleep(150)
   Send("{ENTER}")
   Sleep(350)
EndIf

If Not ListContainsUrl($options, $url) Then
   LogLine("RESULT=FAIL reason=url_not_present_after_add")
   ; Keep Options open for diagnosis. Escape here discards the URL and looks
   ; exactly like the script clicked the window's close button.
   FileClose($gLog)
   Exit 8
EndIf

Local $confirm = FindConfirmButton($options)
If $confirm <> "" Then
   LogLine("clicking confirm control=" & $confirm & " caption=" & ControlGetText($options, "", $confirm))
   ControlClick($options, "", $confirm)
Else
   For $i = 1 To 25
      Local $control = "Button" & $i
      Local $position = ControlGetPos($options, "", $control)
      If @error Or ControlCommand($options, "", $control, "IsVisible", "") <> 1 Then ContinueLoop
      LogLine("visible button " & $control & " caption=" & ControlGetText($options, "", $control))
   Next
   ; Leave the dialog open so a missing/localized button caption cannot
   ; discard the newly entered URL by closing Options with Escape.
   LogLine("RESULT=FAIL reason=confirm_button_not_found_dialog_left_open")
   FileClose($gLog)
   Exit 9
EndIf
If Not WaitOptionsClosed($options, 3000) Then
   LogLine("RESULT=FAIL reason=options_dialog_still_open_after_confirm")
   FileClose($gLog)
   Exit 9
EndIf
LogLine("RESULT=OK url=" & $url)
FileClose($gLog)
Exit 0
