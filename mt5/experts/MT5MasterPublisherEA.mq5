#property strict
#property version   "1.000"
#property description "Publishes the logged-in master terminal position snapshot to the local Go copier publisher."

input string PublisherURL = "http://127.0.0.1:8765";
input string PublisherToken = "";
input int PollIntervalMs = 50;

string EscapeJSON(const string value)
  {
   string escaped = "";
   for(int i = 0; i < StringLen(value); i++)
     {
      ushort c = StringGetCharacter(value, i);
      if(c == 92) escaped += "\\\\";
      else if(c == 34) escaped += "\\\"";
      else if(c == 8) escaped += "\\b";
      else if(c == 12) escaped += "\\f";
      else if(c == 10) escaped += "\\n";
      else if(c == 13) escaped += "\\r";
      else if(c == 9) escaped += "\\t";
      else if(c < 0x20) escaped += StringFormat("\\u%04X", c);
      else escaped += StringSubstr(value, i, 1);
     }
   return escaped;
  }

bool PublishSnapshot()
  {
   string body = "{\"positions\":[";
   int total = PositionsTotal();
   int emitted = 0;
   for(int i = 0; i < total; i++)
     {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0) continue;
      if(emitted > 0) body += ",";
      body += StringFormat("{\"identifier\":%I64d,\"ticket\":%I64u,\"magic\":%I64d,\"symbol\":\"%s\",\"type\":%d,\"volume\":%s,\"price_open\":%s,\"sl\":%s,\"tp\":%s,\"comment\":\"%s\"}",
                           PositionGetInteger(POSITION_IDENTIFIER), ticket,
                           PositionGetInteger(POSITION_MAGIC),
                           EscapeJSON(PositionGetString(POSITION_SYMBOL)),
                           (int)PositionGetInteger(POSITION_TYPE),
                           DoubleToString(PositionGetDouble(POSITION_VOLUME), 8),
                           DoubleToString(PositionGetDouble(POSITION_PRICE_OPEN), 10),
                           DoubleToString(PositionGetDouble(POSITION_SL), 10),
                           DoubleToString(PositionGetDouble(POSITION_TP), 10),
                           EscapeJSON(PositionGetString(POSITION_COMMENT)));
      emitted++;
     }
   body += "]}";

   string url = PublisherURL;
   while(StringLen(url) > 0 && StringSubstr(url, StringLen(url)-1, 1) == "/")
      url = StringSubstr(url, 0, StringLen(url)-1);
   string headers = "Content-Type: application/json\r\nAuthorization: Bearer " + PublisherToken + "\r\n";
   char data[], result[];
   string response_headers;
   int count = StringToCharArray(body, data, 0, WHOLE_ARRAY, CP_UTF8);
   if(count > 0) ArrayResize(data, count - 1);
   ResetLastError();
   int status = WebRequest("POST", url + "/snapshot", headers, 5000, data, result, response_headers);
   if(status < 200 || status >= 300)
     {
      Print("[MT5MasterPublisherEA] snapshot gagal: HTTP ", status, " error ", GetLastError());
      return false;
     }
   return true;
  }

int OnInit()
  {
   if(StringLen(PublisherToken) < 32)
     {
      Print("[MT5MasterPublisherEA] PublisherToken wajib minimal 32 karakter.");
      return INIT_PARAMETERS_INCORRECT;
     }
   if(!EventSetMillisecondTimer(MathMax(10, PollIntervalMs)))
      return INIT_FAILED;
   return INIT_SUCCEEDED;
  }

void OnDeinit(const int reason)
  {
   EventKillTimer();
  }

void OnTimer()
  {
   if(!TerminalInfoInteger(TERMINAL_CONNECTED)) return;
   PublishSnapshot();
  }
