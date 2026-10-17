#property strict
#property version   "1.101"
#property description "Copies all published Algentra positions and reports MT5 account history"

#include <Trade/Trade.mqh>

input string ServerURL = "http://127.0.0.1:8000";
input string AccountToken = "";
input int PollIntervalMs = 500;
input long MagicNumber = 26100301;
input double VolumeMultiplier = 1.0;
input double MaxEntryDeviationPercent = 0.0; // 0 disables the late-entry gate for existing master positions
input double MaxSignalAgeSeconds = 3.0;
input int MaxSlippagePoints = 20;
input string SymbolSuffix = "";
input string SymbolMapCsv = ""; // Optional source-to-broker mapping, e.g. XAUUSDc=GOLD
input bool ManagedMode = false;
input bool DryRun = false; // Validate and report targets without sending, modifying, or closing orders
input string LotMode = "rasio"; // tetap, rasio, saldo
input double FixedLot = 0.01;
input double RatioLot = 1.0;
input double MasterBalance = 10000.0;
input double MaxLotPerOrder = 100.0;
input double MaxLotTotal = 100.0;
input int MaxOpenPositions = 100;
input bool ReverseDirection = false;

CTrade g_trade;
string g_connection_status = "starting";
string g_account_report_status = "waiting for MT5 report";
datetime g_last_log = 0;
datetime g_last_report_attempt = 0;
long g_history_cursor = 0;
long g_history_cursor_msc = 0;
string g_halted_symbols[];
string g_dry_sources[];
string g_dry_symbols[];
double g_dry_volumes[];
bool g_gold_chart_ready = false;
datetime g_gold_chart_last_log = 0;

string TrimTrailingSlash(string value)
  {
   while(StringLen(value) > 0 && StringSubstr(value, StringLen(value) - 1, 1) == "/")
      value = StringSubstr(value, 0, StringLen(value) - 1);
   return value;
  }

string JsonEscape(string value)
  {
   StringReplace(value, "\\", "\\\\");
   StringReplace(value, "\"", "\\\"");
   StringReplace(value, "\r", " ");
   StringReplace(value, "\n", " ");
   return value;
  }

int JsonValueStart(const string json, const string key)
  {
   string needle = "\"" + key + "\"";
   int key_pos = StringFind(json, needle);
   if(key_pos < 0) return -1;
   int colon = StringFind(json, ":", key_pos + StringLen(needle));
   if(colon < 0) return -1;
   int pos = colon + 1;
   while(pos < StringLen(json))
     {
      ushort c = StringGetCharacter(json, pos);
      if(c != 32 && c != 9 && c != 10 && c != 13) break;
      pos++;
     }
   return pos;
  }

string JsonStringValue(const string json, const string key)
  {
   int start = JsonValueStart(json, key);
   if(start < 0 || StringGetCharacter(json, start) != '"') return "";
   int end = StringFind(json, "\"", start + 1);
   if(end < 0) return "";
   return StringSubstr(json, start + 1, end - start - 1);
  }

double JsonNumberValue(const string json, const string key)
  {
   int start = JsonValueStart(json, key);
   if(start < 0 || StringSubstr(json, start, 4) == "null") return 0.0;
   int end = start;
   while(end < StringLen(json))
     {
      ushort c = StringGetCharacter(json, end);
      if((c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E') end++;
      else break;
     }
   if(end == start) return 0.0;
   return StringToDouble(StringSubstr(json, start, end - start));
  }

bool JsonBoolValue(const string json, const string key)
  {
   int start = JsonValueStart(json, key);
   return start >= 0 && StringSubstr(json, start, 4) == "true";
  }

bool JsonObjects(const string json, string &objects[])
  {
   ArrayResize(objects, 0);
   int key_pos = StringFind(json, "\"positions\"");
   if(key_pos < 0) return false;
   int array_start = StringFind(json, "[", key_pos);
   if(array_start < 0) return false;
   int i = array_start + 1;
   while(i < StringLen(json))
     {
      ushort c = StringGetCharacter(json, i);
      if(c == ']') return true;
      if(c == ',' || c == 32 || c == 9 || c == 10 || c == 13) { i++; continue; }
      if(c != '{') return false;
      int object_start = i;
      int depth = 0;
      bool in_string = false;
      bool escaped = false;
      for(; i < StringLen(json); i++)
        {
         ushort ch = StringGetCharacter(json, i);
         if(in_string)
           {
            if(ch == '\\' && !escaped) escaped = true;
            else
              {
               if(ch == '"' && !escaped) in_string = false;
               escaped = false;
              }
            continue;
           }
         if(ch == '"') { in_string = true; continue; }
         if(ch == '{') depth++;
         if(ch == '}') depth--;
         if(depth == 0)
           {
            int size = ArraySize(objects);
            ArrayResize(objects, size + 1);
            objects[size] = StringSubstr(json, object_start, i - object_start + 1);
            i++;
            break;
           }
        }
      if(depth != 0) return false;
     }
   return false;
  }

bool HttpGet(string &response_body)
  {
   if(StringLen(AccountToken) < 32)
     {
      g_connection_status = "account token missing/short";
      return false;
     }
   string terminal_login = IntegerToString((long)AccountInfoInteger(ACCOUNT_LOGIN));
   string terminal_server = AccountInfoString(ACCOUNT_SERVER);
   string headers = "X-Account-Token: " + AccountToken + "\r\n" +
                    "X-MT5-Login: " + terminal_login + "\r\nX-MT5-Server: " + terminal_server + "\r\n";
   char data[];
   char result[];
   string result_headers;
   ArrayResize(data, 0);
   ResetLastError();
   int code = WebRequest("GET", TrimTrailingSlash(ServerURL) + "/api/mt5/follower/positions",
                         headers, 5000, data, result, result_headers);
   response_body = CharArrayToString(result, 0, -1, CP_UTF8);
   if(code < 200 || code >= 300)
     {
      g_connection_status = StringFormat("HTTP %d / error %d", code, GetLastError());
      return false;
     }
   long server_cursor = StringToInteger(JsonStringValue(response_body, "history_cursor"));
   if(server_cursor > g_history_cursor)
      g_history_cursor = server_cursor;
   long server_cursor_msc = (long)JsonNumberValue(response_body, "history_cursor_msc");
   if(server_cursor_msc > g_history_cursor_msc)
      g_history_cursor_msc = server_cursor_msc;
   g_connection_status = "online";
   return true;
  }

string AccountTradeModeText()
  {
   long mode = AccountInfoInteger(ACCOUNT_TRADE_MODE);
   if(mode == ACCOUNT_TRADE_MODE_REAL) return "real";
   if(mode == ACCOUNT_TRADE_MODE_CONTEST) return "contest";
   return "demo";
  }

bool TrySelectBrokerSymbol(const string candidate, string &resolved)
  {
   if(StringLen(candidate) == 0 || !SymbolSelect(candidate, true))
      return false;
   resolved = candidate;
   return true;
  }

bool ResolveBrokerSymbol(const string requested, const string suffix, const string mapping_csv,
                         string &resolved, string &reason)
  {
   resolved = "";
   reason = "";
   string source = requested;
   StringTrimLeft(source);
   StringTrimRight(source);
   if(StringLen(source) == 0)
     {
      reason = "master symbol is empty";
      return false;
     }

   string mapped = "";
   string mappings[];
   int mapping_count = StringSplit(mapping_csv, ',', mappings);
   for(int i = 0; i < mapping_count; i++)
     {
      string item = mappings[i];
      StringTrimLeft(item);
      StringTrimRight(item);
      int equal_at = StringFind(item, "=");
      if(equal_at <= 0)
         continue;
      string from_symbol = StringSubstr(item, 0, equal_at);
      string to_symbol = StringSubstr(item, equal_at + 1);
      StringTrimLeft(from_symbol);
      StringTrimRight(from_symbol);
      StringTrimLeft(to_symbol);
      StringTrimRight(to_symbol);
      string from_upper = from_symbol;
      string source_upper = source;
      StringToUpper(from_upper);
      StringToUpper(source_upper);
      if(from_upper == source_upper && StringLen(to_symbol) > 0)
        {
         mapped = to_symbol;
         break;
        }
     }

   if(mapped != "")
     {
      if(TrySelectBrokerSymbol(mapped, resolved))
         return true;
      if(suffix != "" && TrySelectBrokerSymbol(mapped + suffix, resolved))
         return true;
     }
   if(suffix != "" && TrySelectBrokerSymbol(source + suffix, resolved))
      return true;
   if(TrySelectBrokerSymbol(source, resolved))
      return true;

   string search_for = mapped != "" ? mapped : source;
   string search_upper = search_for;
   StringToUpper(search_upper);
   string match = "";
   string match_upper = "";
   int best_suffix_length = 2147483647;
   int symbols_total = SymbolsTotal(false);
   for(int i = 0; i < symbols_total; i++)
     {
      string candidate = SymbolName(i, false);
      string candidate_upper = candidate;
      StringToUpper(candidate_upper);
      // Exact symbols were tried above. For broker suffixes (XAUUSDc,
      // XAUUSDm, XAUUSD.a, etc.), prefer the shortest suffix and use the
      // symbol name as a stable tie-breaker. Do not select every match:
      // that would clutter Market Watch and leave the chosen instrument
      // dependent on the broker's symbol enumeration order.
      if(candidate_upper == search_upper || StringFind(candidate_upper, search_upper) != 0)
         continue;
      long trade_mode = SymbolInfoInteger(candidate, SYMBOL_TRADE_MODE);
      if(trade_mode == SYMBOL_TRADE_MODE_DISABLED)
         continue;
      int suffix_length = StringLen(candidate) - StringLen(search_for);
      if(suffix_length < best_suffix_length ||
         (suffix_length == best_suffix_length && (match == "" || candidate_upper < match_upper)))
        {
         best_suffix_length = suffix_length;
         match = candidate;
         match_upper = candidate_upper;
        }
     }

   if(match != "")
     {
      if(!SymbolSelect(match, true))
        {
         reason = StringFormat("broker symbol %s was found but could not be shown in Market Watch", match);
         return false;
        }
      resolved = match;
      Print("[MT5FollowerCopyEA] auto-selected broker symbol ", resolved,
            " for ", search_for, " (shortest available suffix)");
      return true;
     }
   reason = StringFormat("no follower symbol matches master symbol %s; configure SymbolSuffix or SymbolMapCsv", source);
   return false;
  }

void EnsureDefaultGoldChart()
  {
   if(g_gold_chart_ready)
      return;
   string gold_symbol, reason;
   if(!ResolveBrokerSymbol("XAUUSD", "", SymbolMapCsv, gold_symbol, reason))
     {
      if(TimeCurrent() - g_gold_chart_last_log >= 15)
        {
         Print("[MT5FollowerCopyEA] gold chart not available yet: ", reason);
         g_gold_chart_last_log = TimeCurrent();
        }
      return;
     }
   if(_Symbol == gold_symbol)
     {
      g_gold_chart_ready = true;
      Print("[MT5FollowerCopyEA] default gold chart: ", gold_symbol, ", M1");
      return;
     }
   ResetLastError();
   if(ChartSetSymbolPeriod(0, gold_symbol, PERIOD_M1))
     {
      g_gold_chart_ready = true;
      Print("[MT5FollowerCopyEA] switching default chart to ", gold_symbol, ", M1");
     }
   else if(TimeCurrent() - g_gold_chart_last_log >= 15)
     {
      Print("[MT5FollowerCopyEA] could not switch default chart to ", gold_symbol,
            "; chart error ", GetLastError());
      g_gold_chart_last_log = TimeCurrent();
     }
  }

string DealEntryText(const ENUM_DEAL_ENTRY entry)
  {
   if(entry == DEAL_ENTRY_IN) return "IN";
   if(entry == DEAL_ENTRY_OUT) return "OUT";
   if(entry == DEAL_ENTRY_OUT_BY) return "OUT_BY";
   if(entry == DEAL_ENTRY_INOUT) return "INOUT";
   return "";
  }

string CurrentOpenPositionIds()
  {
   string ids = "[";
   int count = 0;
   for(int i = PositionsTotal() - 1; i >= 0; i--)
     {
      if(PositionGetTicket(i) == 0) continue;
      long identifier = PositionGetInteger(POSITION_IDENTIFIER);
      if(identifier <= 0) continue;
      if(count > 0) ids += ",";
      ids += StringFormat("\"%I64d\"", identifier);
      count++;
     }
   ids += "]";
   return ids;
  }

bool BuildHistoryDealsJson(const long cursor, const long cursor_msc, string &deals_json, long &batch_cursor)
  {
   deals_json = "[";
   batch_cursor = cursor;
   datetime from_time = cursor_msc > 0
                        ? (datetime)MathMax(0, cursor_msc / 1000 - 1)
                        : TimeCurrent() - 30 * 24 * 60 * 60;
   if(!HistorySelect(from_time, TimeCurrent())) return false;
   int count = 0;
   int total = HistoryDealsTotal();
   for(int i = 0; i < total && count < 100; i++)
     {
      ulong ticket = HistoryDealGetTicket(i);
      if(ticket == 0 || ticket <= (ulong)MathMax(0, cursor)) continue;
      ENUM_DEAL_TYPE type = (ENUM_DEAL_TYPE)HistoryDealGetInteger(ticket, DEAL_TYPE);
      if(type != DEAL_TYPE_BUY && type != DEAL_TYPE_SELL) continue;
      ENUM_DEAL_ENTRY entry_type = (ENUM_DEAL_ENTRY)HistoryDealGetInteger(ticket, DEAL_ENTRY);
      string entry = DealEntryText(entry_type);
      if(entry == "") continue;
      long position_id = HistoryDealGetInteger(ticket, DEAL_POSITION_ID);
      long time_msc = HistoryDealGetInteger(ticket, DEAL_TIME_MSC);
      string symbol = HistoryDealGetString(ticket, DEAL_SYMBOL);
      string action = type == DEAL_TYPE_BUY ? "BUY" : "SELL";
      if(count > 0) deals_json += ",";
      deals_json += StringFormat(
         "{\"deal_ticket\":\"%I64u\",\"position_id\":\"%I64d\",\"time_msc\":%I64d,\"symbol\":\"%s\",\"action\":\"%s\",\"entry\":\"%s\",\"volume\":%s,\"price\":%s,\"profit\":%s,\"commission\":%s,\"swap\":%s,\"fee\":%s}",
         ticket, position_id, time_msc, JsonEscape(symbol), action, entry,
         DoubleToString(HistoryDealGetDouble(ticket, DEAL_VOLUME), 8),
         DoubleToString(HistoryDealGetDouble(ticket, DEAL_PRICE), 8),
         DoubleToString(HistoryDealGetDouble(ticket, DEAL_PROFIT), 8),
         DoubleToString(HistoryDealGetDouble(ticket, DEAL_COMMISSION), 8),
         DoubleToString(HistoryDealGetDouble(ticket, DEAL_SWAP), 8),
         DoubleToString(HistoryDealGetDouble(ticket, DEAL_FEE), 8));
      batch_cursor = (long)ticket;
      count++;
     }
   deals_json += "]";
   return true;
  }

bool PostAccountReport(const string payload, string &response_body)
  {
   string terminal_login = IntegerToString((long)AccountInfoInteger(ACCOUNT_LOGIN));
   string terminal_server = AccountInfoString(ACCOUNT_SERVER);
   string headers = "Content-Type: application/json\r\nX-Account-Token: " + AccountToken + "\r\n" +
                    "X-MT5-Login: " + terminal_login + "\r\nX-MT5-Server: " + terminal_server + "\r\n";
   char data[];
   char result[];
   string result_headers;
   int count = StringToCharArray(payload, data, 0, WHOLE_ARRAY, CP_UTF8);
   if(count > 0) ArrayResize(data, count - 1);
   ResetLastError();
   int code = WebRequest("POST", TrimTrailingSlash(ServerURL) + "/api/mt5/follower/report",
                         headers, 5000, data, result, result_headers);
   response_body = CharArrayToString(result, 0, -1, CP_UTF8);
   if(code < 200 || code >= 300)
     {
      g_account_report_status = StringFormat("report HTTP %d / error %d", code, GetLastError());
      return false;
     }
   long server_cursor = StringToInteger(JsonStringValue(response_body, "history_cursor"));
   if(server_cursor > g_history_cursor) g_history_cursor = server_cursor;
   long server_cursor_msc = (long)JsonNumberValue(response_body, "history_cursor_msc");
   if(server_cursor_msc > g_history_cursor_msc) g_history_cursor_msc = server_cursor_msc;
   g_account_report_status = "saldo dan histori tersinkron";
   return true;
  }

void ReportAccountState()
  {
   datetime now = TimeCurrent();
   if(now - g_last_report_attempt < 5) return;
   g_last_report_attempt = now;
   if(!TerminalInfoInteger(TERMINAL_CONNECTED) || AccountInfoInteger(ACCOUNT_LOGIN) <= 0) return;

   string deals_json;
   long batch_cursor = g_history_cursor;
   if(!BuildHistoryDealsJson(g_history_cursor, g_history_cursor_msc, deals_json, batch_cursor))
     {
      g_account_report_status = "riwayat MT5 belum dapat dibaca";
      return;
     }
   string currency = JsonEscape(AccountInfoString(ACCOUNT_CURRENCY));
   double floating_profit = 0.0;
   for(int i = PositionsTotal() - 1; i >= 0; i--)
     {
      if(PositionGetTicket(i) == 0) continue;
      floating_profit += PositionGetDouble(POSITION_PROFIT) + PositionGetDouble(POSITION_SWAP);
     }
   string trade_mode = AccountTradeModeText();
   string payload = StringFormat(
      "{\"balance\":%s,\"equity\":%s,\"floating_profit\":%s,\"margin\":%s,\"free_margin\":%s,\"currency\":\"%s\",\"trade_mode\":\"%s\",\"allow_live_trading\":%s,\"terminal_trade_allowed\":%s,\"expert_trade_allowed\":%s,\"open_position_ids\":%s,\"deals\":%s}",
      DoubleToString(AccountInfoDouble(ACCOUNT_BALANCE), 8),
      DoubleToString(AccountInfoDouble(ACCOUNT_EQUITY), 8),
      DoubleToString(floating_profit, 8),
      DoubleToString(AccountInfoDouble(ACCOUNT_MARGIN), 8),
      DoubleToString(AccountInfoDouble(ACCOUNT_MARGIN_FREE), 8),
      currency, trade_mode,
      trade_mode == "real" ? "true" : "false",
      (TerminalInfoInteger(TERMINAL_TRADE_ALLOWED) && AccountInfoInteger(ACCOUNT_TRADE_ALLOWED)) ? "true" : "false",
      (MQLInfoInteger(MQL_TRADE_ALLOWED) && AccountInfoInteger(ACCOUNT_TRADE_EXPERT)) ? "true" : "false",
      CurrentOpenPositionIds(), deals_json);
   string response;
   PostAccountReport(payload, response);
  }

string Marker(const string source_ticket)
  {
   return "ACM:" + source_ticket;
  }

bool FindCopiedPositions(const string source_ticket, ulong &tickets[], double &volume)
  {
   ArrayResize(tickets, 0);
   volume = 0.0;
   string marker = Marker(source_ticket);
   for(int i = PositionsTotal() - 1; i >= 0; i--)
     {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0) continue;
      if(PositionGetInteger(POSITION_MAGIC) != MagicNumber) continue;
      if(PositionGetString(POSITION_COMMENT) != marker) continue;
      int size = ArraySize(tickets);
      ArrayResize(tickets, size + 1);
      tickets[size] = ticket;
      volume += PositionGetDouble(POSITION_VOLUME);
     }
   return ArraySize(tickets) > 0;
  }

int VolumePrecision(const double step)
  {
   for(int digits = 0; digits <= 8; digits++)
      if(MathAbs(step - NormalizeDouble(step, digits)) < 0.000000001) return digits;
   return 8;
  }

double NormalizeLots(const string symbol, const double requested)
  {
   double minimum = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);
   double maximum = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MAX);
   double step = SymbolInfoDouble(symbol, SYMBOL_VOLUME_STEP);
   if(minimum <= 0.0 || maximum <= 0.0 || step <= 0.0 || requested < minimum)
      return 0.0;
   double capped_request = MathMin(requested, maximum);
   double lots = MathFloor(capped_request / step + 0.0000000001) * step;
   double float_tolerance = MathMax(0.000000000000001,
                                    MathAbs(capped_request) * 2.220446049250313e-16 * 2.0);
   if(lots > capped_request + float_tolerance) lots -= step;
   if(lots + step * 0.000000001 < minimum) return 0.0;
   if(lots > maximum) lots = MathFloor(maximum / step + 0.0000000001) * step;
   return NormalizeDouble(lots, VolumePrecision(step));
  }

void AdjustStops(const string symbol, const string action, const double price, double &sl, double &tp)
  {
   double point = SymbolInfoDouble(symbol, SYMBOL_POINT);
   double distance = MathMax((double)SymbolInfoInteger(symbol, SYMBOL_TRADE_STOPS_LEVEL),
                             (double)SymbolInfoInteger(symbol, SYMBOL_TRADE_FREEZE_LEVEL)) * point;
   if(distance > 0.0)
     {
      if(action == "BUY")
        {
         if(sl > 0.0 && sl > price - distance) sl = price - distance;
         if(tp > 0.0 && tp < price + distance) tp = price + distance;
        }
      else
        {
         if(sl > 0.0 && sl < price + distance) sl = price + distance;
         if(tp > 0.0 && tp > price - distance) tp = price - distance;
        }
     }
   int digits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);
   sl = NormalizeDouble(sl, digits);
   tp = NormalizeDouble(tp, digits);
  }

int CountManagedPositions()
  {
   int count = 0;
   for(int i = PositionsTotal() - 1; i >= 0; i--)
      if(PositionGetTicket(i) > 0 && PositionGetInteger(POSITION_MAGIC) == MagicNumber)
         count++;
   return count;
  }

double TargetVolume(const double master_lots)
  {
   double requested = master_lots * VolumeMultiplier;
   if(ManagedMode)
     {
      if(LotMode == "tetap") requested = FixedLot;
      else if(LotMode == "rasio") requested = master_lots * RatioLot;
      else if(LotMode == "saldo")
        {
         if(MasterBalance <= 0.0) return 0.0;
         requested = master_lots * AccountInfoDouble(ACCOUNT_BALANCE) / MasterBalance;
        }
      else return 0.0;
      if(MaxLotPerOrder <= 0.0) return 0.0;
      requested = MathMin(requested, MaxLotPerOrder);
     }
   return requested;
  }

bool RiskAllowsIncrease(const string symbol, const double additional_lots, const bool opens_position)
  {
   if(!ManagedMode) return true;
   if(MaxOpenPositions <= 0 || MaxLotTotal <= 0.0 || additional_lots <= 0.0) return false;
   int open_count = PositionsTotal();
   double total_lots = 0.0;
   for(int i = PositionsTotal() - 1; i >= 0; i--)
     {
      if(PositionGetTicket(i) == 0) continue;
      total_lots += PositionGetDouble(POSITION_VOLUME);
     }
   if(DryRun)
     {
      open_count += ArraySize(g_dry_sources);
      for(int i = 0; i < ArraySize(g_dry_volumes); i++) total_lots += g_dry_volumes[i];
     }
   if(opens_position && open_count >= MaxOpenPositions) return false;
   return total_lots + additional_lots <= MaxLotTotal + 0.0000000001;
  }

string NettingExposureKey(const string symbol)
  {
   return StringFormat("AGNE_%I64d_%I64d_%s", AccountInfoInteger(ACCOUNT_LOGIN), MagicNumber, symbol);
  }

string VirtualStopKey(const string source_ticket)
  {
   return StringFormat("AGNS_%I64d_%I64d_%s", AccountInfoInteger(ACCOUNT_LOGIN), MagicNumber, source_ticket);
  }

string CopiedSourceKey(const string source_ticket)
  {
   return StringFormat("AGNC_%I64d_%I64d_%s", AccountInfoInteger(ACCOUNT_LOGIN), MagicNumber, source_ticket);
  }

bool IsSymbolHalted(const string symbol)
  {
   for(int i = 0; i < ArraySize(g_halted_symbols); i++)
      if(g_halted_symbols[i] == symbol) return true;
   return false;
  }

void HaltSymbol(const string symbol, const string reason)
  {
   if(!IsSymbolHalted(symbol))
     {
      int size = ArraySize(g_halted_symbols);
      ArrayResize(g_halted_symbols, size + 1);
      g_halted_symbols[size] = symbol;
     }
   Print("[MT5FollowerCopyEA] Netting ", symbol, " halted: ", reason);
  }

double FloorVolumeStep(const double requested, const double step)
  {
   if(step <= 0.0 || requested <= 0.0) return 0.0;
   return NormalizeDouble(MathFloor(requested / step + 0.0000000001) * step, VolumePrecision(step));
  }

bool ReadNetExposure(const string symbol, double &signed_volume, ulong &ticket)
  {
   signed_volume = 0.0;
   ticket = 0;
   if(!PositionSelect(symbol)) return true;
   ticket = (ulong)PositionGetInteger(POSITION_TICKET);
   double volume = PositionGetDouble(POSITION_VOLUME);
   long side = PositionGetInteger(POSITION_TYPE);
   signed_volume = side == POSITION_TYPE_BUY ? volume : -volume;
   return true;
  }

bool IsVirtualStop(const string source_ticket)
  {
   string key = VirtualStopKey(source_ticket);
   return GlobalVariableCheck(key) && GlobalVariableGet(key) > 0.5;
  }

void MarkVirtualStop(const string source_ticket)
  {
   GlobalVariableSet(VirtualStopKey(source_ticket), 1.0);
  }

bool IsSourceTicket(const string ticket, string &source_tickets[]);
void PollNetting(string &objects[], const double copy_after)
  {
   string source_tickets[];
   ArrayResize(source_tickets, ArraySize(objects));
   for(int i = 0; i < ArraySize(objects); i++)
      source_tickets[i] = JsonStringValue(objects[i], "ticket");

   // A virtual stop suppresses one master position until its close signal is
   // consumed; it must not be reopened by the unchanged master snapshot.
   for(int i = GlobalVariablesTotal() - 1; i >= 0; i--)
     {
      string name = GlobalVariableName(i);
      string prefix = StringFormat("AGNS_%I64d_%I64d_", AccountInfoInteger(ACCOUNT_LOGIN), MagicNumber);
      if(StringFind(name, prefix) != 0) continue;
      string source_ticket = StringSubstr(name, StringLen(prefix));
      if(!IsSourceTicket(source_ticket, source_tickets)) GlobalVariableDel(name);
     }
   string copied_prefix = StringFormat("AGNC_%I64d_%I64d_", AccountInfoInteger(ACCOUNT_LOGIN), MagicNumber);
   for(int i = GlobalVariablesTotal() - 1; i >= 0; i--)
     {
      string name = GlobalVariableName(i);
      if(StringFind(name, copied_prefix) != 0) continue;
      string source_ticket = StringSubstr(name, StringLen(copied_prefix));
      if(!IsSourceTicket(source_ticket, source_tickets)) GlobalVariableDel(name);
     }

   string symbols[];
   double targets[];
   double gross_by_symbol[];
   for(int i = 0; i < ArraySize(objects); i++)
     {
      string object = objects[i];
      string source_ticket = JsonStringValue(object, "ticket");
      string source_symbol = JsonStringValue(object, "symbol");
      string action = JsonStringValue(object, "action");
      double master_lots = JsonNumberValue(object, "lots");
      double opened_at = JsonNumberValue(object, "opened_at");
      if(source_ticket == "" || source_symbol == "" || master_lots <= 0.0 ||
         (action != "BUY" && action != "SELL")) continue;
      if(IsVirtualStop(source_ticket)) continue;
      if(ReverseDirection) action = action == "BUY" ? "SELL" : "BUY";
      string symbol, reason;
      if(!ResolveBrokerSymbol(source_symbol, SymbolSuffix, SymbolMapCsv, symbol, reason)) continue;
      string copied_key = CopiedSourceKey(source_ticket);
      if(opened_at > 0.0 && opened_at <= copy_after && !GlobalVariableCheck(copied_key)) continue;
      if(opened_at > 0.0 && !GlobalVariableCheck(copied_key) && TimeGMT() - opened_at > MaxSignalAgeSeconds) continue;
      if(!SymbolSelect(symbol, true)) continue;
      MqlTick tick;
      if(!SymbolInfoTick(symbol, tick) || tick.bid <= 0.0 || tick.ask <= 0.0) continue;
      double sl = JsonNumberValue(object, "sl");
      double tp = JsonNumberValue(object, "tp");
      double stop_price = action == "BUY" ? tick.bid : tick.ask;
      bool stopped = (action == "BUY" && ((sl > 0.0 && stop_price <= sl) || (tp > 0.0 && stop_price >= tp))) ||
                     (action == "SELL" && ((sl > 0.0 && stop_price >= sl) || (tp > 0.0 && stop_price <= tp)));
      if(stopped)
        {
         MarkVirtualStop(source_ticket);
         Print("[MT5FollowerCopyEA] virtual SL/TP triggered for source ", source_ticket, " on ", symbol);
         continue;
        }
      double volume = NormalizeLots(symbol, TargetVolume(master_lots));
      if(volume <= 0.0) continue;
      int index = -1;
      for(int j = 0; j < ArraySize(symbols); j++)
         if(symbols[j] == symbol) { index = j; break; }
      if(index < 0)
        {
         index = ArraySize(symbols);
         ArrayResize(symbols, index + 1);
         ArrayResize(targets, index + 1);
         ArrayResize(gross_by_symbol, index + 1);
         symbols[index] = symbol;
         targets[index] = 0.0;
         gross_by_symbol[index] = 0.0;
        }
      double direction = action == "BUY" ? 1.0 : -1.0;
      targets[index] += direction * volume;
      gross_by_symbol[index] += volume;
     }

   // Revisit symbols saved by earlier polls, including symbols whose final
   // master position just closed, so a zero target closes the net position.
   string exposure_prefix = StringFormat("AGNE_%I64d_%I64d_", AccountInfoInteger(ACCOUNT_LOGIN), MagicNumber);
   for(int i = GlobalVariablesTotal() - 1; i >= 0; i--)
     {
      string name = GlobalVariableName(i);
      if(StringFind(name, exposure_prefix) != 0) continue;
      string symbol = StringSubstr(name, StringLen(exposure_prefix));
      if(StringLen(symbol) == 0) continue;
      bool exists = false;
      for(int j = 0; j < ArraySize(symbols); j++)
         if(symbols[j] == symbol) { exists = true; break; }
      if(exists) continue;
      int index = ArraySize(symbols);
      ArrayResize(symbols, index + 1);
      ArrayResize(targets, index + 1);
      ArrayResize(gross_by_symbol, index + 1);
      symbols[index] = symbol;
      targets[index] = 0.0;
      gross_by_symbol[index] = 0.0;
     }

   // Include existing broker positions even if the last managed state file is
   // missing. With no saved expected exposure, the fail-closed check below
   // pauses the symbol instead of silently taking over a manual position.
   for(int i = PositionsTotal() - 1; i >= 0; i--)
     {
      ulong position_ticket = PositionGetTicket(i);
      if(position_ticket == 0) continue;
      string symbol = PositionGetString(POSITION_SYMBOL);
      bool exists = false;
      for(int j = 0; j < ArraySize(symbols); j++)
         if(symbols[j] == symbol) { exists = true; break; }
      if(exists) continue;
      int index = ArraySize(symbols);
      ArrayResize(symbols, index + 1);
      ArrayResize(targets, index + 1);
      ArrayResize(gross_by_symbol, index + 1);
      symbols[index] = symbol;
      targets[index] = 0.0;
      gross_by_symbol[index] = 0.0;
     }

   int exposed_symbols = 0;
   double gross_total = 0.0;
   for(int i = 0; i < ArraySize(symbols); i++)
     {
      gross_total += gross_by_symbol[i];
      if(gross_by_symbol[i] > 0.00000001) exposed_symbols++;
     }
   if(ManagedMode && (exposed_symbols > MaxOpenPositions || gross_total > MaxLotTotal + 0.0000000001))
     {
      g_connection_status = "Netting target exceeds managed risk limits";
      Print("[MT5FollowerCopyEA] managed Netting risk limit exceeded; no target changes sent");
      return;
     }

   if(DryRun)
     {
      if(TimeCurrent() - g_last_log >= 15)
        {
         Print("[MT5FollowerCopyEA] dry run: Netting targets checked; no broker orders sent");
         g_last_log = TimeCurrent();
        }
      g_connection_status = "dry run (Netting)";
      return;
     }

   for(int i = 0; i < ArraySize(symbols); i++)
     {
      string symbol = symbols[i];
      if(IsSymbolHalted(symbol)) continue;
      double actual;
      ulong ticket;
      if(!ReadNetExposure(symbol, actual, ticket))
        {
         HaltSymbol(symbol, "cannot read actual position");
         continue;
        }
      string key = NettingExposureKey(symbol);
      double step = SymbolInfoDouble(symbol, SYMBOL_VOLUME_STEP);
      double minimum = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);
      double tolerance = MathMax(step * 0.25, 0.00000001);
      double expected = GlobalVariableCheck(key) ? GlobalVariableGet(key) : 0.0;
      if(MathAbs(actual - expected) > tolerance)
        {
         HaltSymbol(symbol, StringFormat("actual %.8g differs from saved copier exposure %.8g", actual, expected));
         continue;
        }
      double delta = targets[i] - actual;
      if(MathAbs(delta) < minimum - tolerance)
        {
         GlobalVariableSet(key, actual);
         continue;
        }
      double volume = FloorVolumeStep(MathAbs(delta), step);
      if(volume < minimum - tolerance)
        {
         Print("[MT5FollowerCopyEA] Netting adjustment for ", symbol, " is below broker minimum");
         continue;
        }
      g_trade.SetTypeFillingBySymbol(symbol);
      bool sent = delta > 0.0
                  ? g_trade.Buy(volume, symbol, 0.0, 0.0, 0.0, "Algentra managed")
                  : g_trade.Sell(volume, symbol, 0.0, 0.0, 0.0, "Algentra managed");
      bool accepted = sent && (g_trade.ResultRetcode() == TRADE_RETCODE_DONE ||
                               g_trade.ResultRetcode() == TRADE_RETCODE_DONE_PARTIAL ||
                               g_trade.ResultRetcode() == TRADE_RETCODE_PLACED);
      double after;
      ulong after_ticket;
      if(!ReadNetExposure(symbol, after, after_ticket))
        {
         HaltSymbol(symbol, "cannot verify broker result");
         continue;
        }
      if(!accepted && MathAbs(after - actual) <= tolerance)
        {
         Print("[MT5FollowerCopyEA] Netting order failed for ", symbol, ": ", g_trade.ResultRetcodeDescription());
         continue;
        }
      if(MathAbs(after - actual) > tolerance && MathAbs(targets[i] - after) >= minimum - tolerance && !accepted)
        {
         HaltSymbol(symbol, "broker changed exposure after an unconfirmed order");
         continue;
        }
      GlobalVariableSet(key, after);
      if(MathAbs(after - targets[i]) > tolerance)
         Print("[MT5FollowerCopyEA] Netting ", symbol, " partial adjustment; will retry on next poll");
      else
        {
         for(int j = 0; j < ArraySize(objects); j++)
           {
            string object_symbol = JsonStringValue(objects[j], "symbol");
            string resolved_symbol, resolve_reason;
            if(!ResolveBrokerSymbol(object_symbol, SymbolSuffix, SymbolMapCsv, resolved_symbol, resolve_reason) || resolved_symbol != symbol)
               continue;
            double opened_at = JsonNumberValue(objects[j], "opened_at");
            string source_ticket = JsonStringValue(objects[j], "ticket");
            if(opened_at > 0.0 && opened_at <= copy_after && !GlobalVariableCheck(CopiedSourceKey(source_ticket)))
               continue;
            GlobalVariableSet(CopiedSourceKey(source_ticket), 1.0);
           }
        }
     }
   g_connection_status = "copying / live (Netting)";
  }

bool IsSourceTicket(const string ticket, string &source_tickets[])
  {
   for(int i = 0; i < ArraySize(source_tickets); i++)
      if(source_tickets[i] == ticket) return true;
   return false;
  }

void CloseMissingCopies(string &source_tickets[])
  {
   if(DryRun) return;
   for(int i = PositionsTotal() - 1; i >= 0; i--)
     {
      ulong local_ticket = PositionGetTicket(i);
      if(local_ticket == 0 || PositionGetInteger(POSITION_MAGIC) != MagicNumber) continue;
      string comment = PositionGetString(POSITION_COMMENT);
      if(StringFind(comment, "ACM:") != 0) continue;
      string source_ticket = StringSubstr(comment, 4);
      if(IsSourceTicket(source_ticket, source_tickets)) continue;
      if(g_trade.PositionClose(local_ticket, MaxSlippagePoints))
         Print("[MT5FollowerCopyEA] source position ", source_ticket, " closed; Copy Trading position ", local_ticket, " closed");
      else
         Print("[MT5FollowerCopyEA] failed to close Copy Trading position ", local_ticket, ": ", g_trade.ResultRetcodeDescription());
     }
   string copied_prefix = StringFormat("AGNC_%I64d_%I64d_", AccountInfoInteger(ACCOUNT_LOGIN), MagicNumber);
   for(int i = GlobalVariablesTotal() - 1; i >= 0; i--)
     {
      string name = GlobalVariableName(i);
      if(StringFind(name, copied_prefix) != 0) continue;
      string source_ticket = StringSubstr(name, StringLen(copied_prefix));
      if(!IsSourceTicket(source_ticket, source_tickets)) GlobalVariableDel(name);
     }
  }

bool OpenCopy(const string source_ticket, const string symbol, const string action,
              const double lots, const double sl, const double tp)
  {
   if(DryRun)
     {
      Print("[MT5FollowerCopyEA] dry run: would copy ", action, " ", DoubleToString(lots, 2), " ", symbol,
            " from source ", source_ticket);
      return true;
     }
   if(GlobalVariableCheck(CopiedSourceKey(source_ticket)))
     {
      Print("[MT5FollowerCopyEA] source ", source_ticket, " was previously copied; an absent local position will not be reopened automatically");
      return false;
     }
   MqlTick quote;
   if(!SymbolSelect(symbol, true) || !SymbolInfoTick(symbol, quote)) return false;
   g_trade.SetTypeFillingBySymbol(symbol);
   bool sent = action == "BUY"
               ? g_trade.Buy(lots, symbol, 0.0, sl, tp, Marker(source_ticket))
               : g_trade.Sell(lots, symbol, 0.0, sl, tp, Marker(source_ticket));
   if(!sent || (g_trade.ResultRetcode() != TRADE_RETCODE_DONE &&
                g_trade.ResultRetcode() != TRADE_RETCODE_DONE_PARTIAL &&
                g_trade.ResultRetcode() != TRADE_RETCODE_PLACED))
     {
      Print("[MT5FollowerCopyEA] copy open failed for source ", source_ticket, ": ", g_trade.ResultRetcodeDescription());
      return false;
     }
   Print("[MT5FollowerCopyEA] copied ", action, " ", DoubleToString(lots, 2), " ", symbol,
         " from source ", source_ticket);
   GlobalVariableSet(CopiedSourceKey(source_ticket), 1.0);
   return true;
  }

void ReconcileVolumeAndStops(const string source_ticket, const string symbol, const string action,
                             const double requested_lots, const double sl, const double tp)
  {
   ulong local_tickets[];
   double current_volume = 0.0;
   FindCopiedPositions(source_ticket, local_tickets, current_volume);
   double target_volume = NormalizeLots(symbol, TargetVolume(requested_lots));
   if(target_volume <= 0.0)
     {
      Print("[MT5FollowerCopyEA] source ", source_ticket, " skipped: Copy Trading lot is outside broker limits");
      return;
     }
   if(DryRun)
     {
      for(int i=0;i<ArraySize(g_dry_sources);i++)
         if(g_dry_sources[i]==source_ticket) return;
      if(!RiskAllowsIncrease(symbol,target_volume,true))
        {
         Print("[MT5FollowerCopyEA] dry run: risk limits prevent opening source ",source_ticket);
         return;
        }
      int dry_index=ArraySize(g_dry_sources);
      ArrayResize(g_dry_sources,dry_index+1);
      ArrayResize(g_dry_symbols,dry_index+1);
      ArrayResize(g_dry_volumes,dry_index+1);
      g_dry_sources[dry_index]=source_ticket;
      g_dry_symbols[dry_index]=symbol;
      g_dry_volumes[dry_index]=target_volume;
      if(TimeCurrent() - g_last_log >= 15)
        {
         Print("[MT5FollowerCopyEA] dry run: target ", action, " ", DoubleToString(target_volume, 2), " ", symbol,
               " from source ", source_ticket, "; no broker orders sent");
         g_last_log = TimeCurrent();
        }
      return;
     }
   double step = SymbolInfoDouble(symbol, SYMBOL_VOLUME_STEP);
   double tolerance = step * 0.51;

   if(ArraySize(local_tickets) == 0)
     {
      if(!RiskAllowsIncrease(symbol, target_volume, true))
        {
         Print("[MT5FollowerCopyEA] risk limits prevent opening source ", source_ticket);
         return;
        }
      OpenCopy(source_ticket, symbol, action, target_volume, sl, tp);
      return;
     }

   for(int i = 0; i < ArraySize(local_tickets); i++)
     {
      ulong ticket = local_tickets[i];
      if(!PositionSelectByTicket(ticket)) continue;
      double old_sl = PositionGetDouble(POSITION_SL);
      double old_tp = PositionGetDouble(POSITION_TP);
      double point = SymbolInfoDouble(symbol, SYMBOL_POINT);
      bool sl_changed = MathAbs(old_sl - sl) > point * 0.5;
      bool tp_changed = MathAbs(old_tp - tp) > point * 0.5;
      if((sl_changed || tp_changed) && !g_trade.PositionModify(ticket, sl, tp))
         Print("[MT5FollowerCopyEA] stop update failed for Copy Trading position ", ticket, ": ", g_trade.ResultRetcodeDescription());
     }

   current_volume = 0.0;
   for(int i = 0; i < ArraySize(local_tickets); i++)
      if(PositionSelectByTicket(local_tickets[i])) current_volume += PositionGetDouble(POSITION_VOLUME);

   if(target_volume + tolerance < current_volume)
     {
      double to_reduce = current_volume - target_volume;
      for(int i = 0; i < ArraySize(local_tickets) && to_reduce > tolerance; i++)
        {
         ulong ticket = local_tickets[i];
         if(!PositionSelectByTicket(ticket)) continue;
         double position_volume = PositionGetDouble(POSITION_VOLUME);
         double minimum = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);
         double reduction = MathMin(to_reduce, position_volume);
         if(reduction >= position_volume - tolerance)
           {
            if(g_trade.PositionClose(ticket, MaxSlippagePoints)) to_reduce -= position_volume;
           }
         else
           {
            reduction = NormalizeLots(symbol, reduction);
            if(reduction >= minimum && g_trade.PositionClosePartial(ticket, reduction, MaxSlippagePoints))
               to_reduce -= reduction;
           }
        }
     }
   else if(target_volume > current_volume + tolerance)
     {
      double extra = NormalizeLots(symbol, target_volume - current_volume);
      if(extra > 0.0 && RiskAllowsIncrease(symbol, extra, false)) OpenCopy(source_ticket, symbol, action, extra, sl, tp);
     }
  }

void PollAndCopy()
  {
   if(!TerminalInfoInteger(TERMINAL_CONNECTED) || AccountInfoInteger(ACCOUNT_LOGIN) <= 0)
     {
      g_connection_status = "trading terminal disconnected";
      return;
     }
   string response;
   if(!HttpGet(response))
     {
      if(TimeCurrent() - g_last_log >= 15)
        {
         Print("[MT5FollowerCopyEA] backend request failed: ", g_connection_status);
         g_last_log = TimeCurrent();
        }
      return;
     }
   if(!JsonBoolValue(response, "active"))
     {
      g_connection_status = "waiting for fresh master snapshot";
      return;
     }

   string objects[];
   if(!JsonObjects(response, objects))
     {
      g_connection_status = "invalid master snapshot";
      return;
     }
   double copy_after = JsonNumberValue(response, "copy_after");
   string source_tickets[];
   int total = ArraySize(objects);
   ArrayResize(source_tickets, total);
   for(int i = 0; i < total; i++)
      source_tickets[i] = JsonStringValue(objects[i], "ticket");

   if(DryRun)
     {
      ArrayResize(g_dry_sources,0);
      ArrayResize(g_dry_symbols,0);
      ArrayResize(g_dry_volumes,0);
     }

   if(!TerminalInfoInteger(TERMINAL_TRADE_ALLOWED) || !MQLInfoInteger(MQL_TRADE_ALLOWED) ||
      !AccountInfoInteger(ACCOUNT_TRADE_ALLOWED) || !AccountInfoInteger(ACCOUNT_TRADE_EXPERT))
     {
      g_connection_status = "connected / trading not allowed by terminal";
      return;
     }
   g_trade.SetExpertMagicNumber(MagicNumber);
   g_trade.SetDeviationInPoints(MaxSlippagePoints);
   if(AccountInfoInteger(ACCOUNT_MARGIN_MODE) != ACCOUNT_MARGIN_MODE_RETAIL_HEDGING)
     {
      PollNetting(objects, copy_after);
      return;
     }

   for(int i = 0; i < total; i++)
     {
      string object = objects[i];
      string source_ticket = JsonStringValue(object, "ticket");
      string source_symbol = JsonStringValue(object, "symbol");
      string symbol;
      string symbol_error;
      string action = JsonStringValue(object, "action");
      double lots = JsonNumberValue(object, "lots");
      double entry = JsonNumberValue(object, "entry_price");
      double sl = JsonNumberValue(object, "sl");
      double tp = JsonNumberValue(object, "tp");
      if(source_ticket == "" || source_symbol == "" || (action != "BUY" && action != "SELL") || lots <= 0.0)
         continue;
      if(ReverseDirection) action = action == "BUY" ? "SELL" : "BUY";
      if(!ResolveBrokerSymbol(source_symbol, SymbolSuffix, SymbolMapCsv, symbol, symbol_error))
        {
         if(TimeCurrent() - g_last_log >= 15)
           {
            Print("[MT5FollowerCopyEA] source ", source_ticket, " waiting: ", symbol_error);
            g_last_log = TimeCurrent();
           }
         continue;
        }

      ulong local_tickets[];
      double local_volume = 0.0;
      bool already_copied = FindCopiedPositions(source_ticket, local_tickets, local_volume);
      double opened_at = JsonNumberValue(object, "opened_at");
      if(!already_copied && ManagedMode && copy_after > 0.0 && opened_at > 0.0 && opened_at <= copy_after)
         continue;
      if(!already_copied && ManagedMode && opened_at > 0.0 && TimeGMT() - opened_at > MaxSignalAgeSeconds)
         continue;
      MqlTick quote;
      if(!SymbolInfoTick(symbol, quote) || quote.ask <= 0.0 || quote.bid <= 0.0)
        {
         if(TimeCurrent() - g_last_log >= 15)
           {
            Print("[MT5FollowerCopyEA] source ", source_ticket, " waiting: no live quote for ", symbol);
            g_last_log = TimeCurrent();
           }
         continue;
        }
      double current_price = action == "BUY" ? quote.ask : quote.bid;
      if(!already_copied && MaxEntryDeviationPercent > 0.0 && entry > 0.0 &&
         MathAbs(current_price - entry) / entry * 100.0 > MaxEntryDeviationPercent)
        {
         if(TimeCurrent() - g_last_log >= 15)
           {
            Print("[MT5FollowerCopyEA] source ", source_ticket, " skipped: entry price deviation exceeded");
            g_last_log = TimeCurrent();
           }
         continue;
        }
      AdjustStops(symbol, action, current_price, sl, tp);
      ReconcileVolumeAndStops(source_ticket, symbol, action, lots, sl, tp);
     }
   CloseMissingCopies(source_tickets);
   g_connection_status = DryRun ? "dry run" : "copying / live";
  }

int OnInit()
  {
   if(PollIntervalMs < 20 || StringLen(AccountToken) < 32 || VolumeMultiplier <= 0.0 ||
      MaxEntryDeviationPercent < 0.0 || MaxSignalAgeSeconds < 0.0)
      return INIT_PARAMETERS_INCORRECT;
   if(ManagedMode && (MaxOpenPositions <= 0 || MaxLotTotal <= 0.0 || MaxLotPerOrder <= 0.0 ||
      (LotMode != "tetap" && LotMode != "rasio" && LotMode != "saldo") ||
      (LotMode == "tetap" && FixedLot <= 0.0) || (LotMode == "rasio" && RatioLot <= 0.0) ||
      (LotMode == "saldo" && MasterBalance <= 0.0)))
      return INIT_PARAMETERS_INCORRECT;
   EnsureDefaultGoldChart();
   EventSetMillisecondTimer(PollIntervalMs);
   ReportAccountState();
   PollAndCopy();
   Print("[MT5FollowerCopyEA] initialized; DryRun=", DryRun, ". Allow ServerURL in MT5 WebRequest options.");
   return INIT_SUCCEEDED;
  }

void OnDeinit(const int reason)
  {
   EventKillTimer();
   Comment("");
   Print("[MT5FollowerCopyEA] stopped, reason ", reason);
  }

void OnTimer()
  {
   EnsureDefaultGoldChart();
   ReportAccountState();
   PollAndCopy();
   Comment("Algentra MT5 Copy Trading\nBackend: ", g_connection_status,
           "\nManaged positions: ", CountManagedPositions(),
           "\nSaldo: ", AccountInfoString(ACCOUNT_CURRENCY), " ", DoubleToString(AccountInfoDouble(ACCOUNT_BALANCE), 2),
           "\nCopying permission: ", DryRun ? "DRY RUN" : "ON",
           "\nLaporan akun: ", g_account_report_status);
  }
