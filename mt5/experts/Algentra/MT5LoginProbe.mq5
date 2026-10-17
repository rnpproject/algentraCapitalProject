#property strict
#property version   "1.000"
#property description "Reports isolated MT5 terminal login and trade readiness to the Go managed worker."

string EscapeJSON(string value)
  {
   StringReplace(value, "\\", "\\\\");
   StringReplace(value, "\"", "\\\"");
   StringReplace(value, "\r", " ");
   StringReplace(value, "\n", " ");
   return value;
  }

void WriteStatus(const bool ready, const bool valid, const string message)
  {
   int file = FileOpen("algentra-login-probe.json", FILE_WRITE|FILE_TXT|FILE_ANSI);
   if(file == INVALID_HANDLE) return;
   string safe_message = EscapeJSON(message);
   string server = EscapeJSON(AccountInfoString(ACCOUNT_SERVER));
   FileWriteString(file, StringFormat("{\"ready\":%s,\"valid\":%s,\"message\":\"%s\",\"login\":%I64d,\"server\":\"%s\"}",
                                      ready ? "true" : "false",
                                      valid ? "true" : "false",
                                      safe_message,
                                      AccountInfoInteger(ACCOUNT_LOGIN),
                                      server));
   FileFlush(file);
   FileClose(file);
  }

int OnInit()
  {
   WriteStatus(false, false, "Menunggu koneksi broker MT5.");
   if(!EventSetTimer(1)) return INIT_FAILED;
   return INIT_SUCCEEDED;
  }

void OnDeinit(const int reason)
  {
   EventKillTimer();
  }

void OnTimer()
  {
   if(!TerminalInfoInteger(TERMINAL_CONNECTED) || AccountInfoInteger(ACCOUNT_LOGIN) <= 0)
     {
      WriteStatus(false, false, "Menunggu koneksi broker MT5.");
      return;
     }
   if(!AccountInfoInteger(ACCOUNT_TRADE_ALLOWED))
     {
      WriteStatus(true, false, "Login broker berhasil, tetapi broker menolak izin trading akun ini.");
      return;
     }
   if(!TerminalInfoInteger(TERMINAL_TRADE_ALLOWED))
     {
      WriteStatus(true, false, "Login berhasil, tetapi izin Auto Trading terminal belum aktif.");
      return;
     }
   WriteStatus(true, true, "Login broker berhasil dan terminal siap untuk copy trading.");
  }
