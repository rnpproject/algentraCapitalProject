package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (w *worker) prepareManagedEA(j job, token, terminalDir, name string) error {
	source := filepath.Join(w.cfg.projectRoot, "mt5", "experts", "MT5FollowerCopyEA.ex5")
	if err := copyFile(source, filepath.Join(terminalDir, "MQL5", "Experts", "MT5FollowerCopyEA.ex5")); err != nil {
		return fmt.Errorf("binary MT5FollowerCopyEA.ex5 tidak dapat dipasang; compile mt5/experts/MT5FollowerCopyEA.mq5: %w", err)
	}
	presetName := name + ".set"
	presetPath := filepath.Join(terminalDir, "MQL5", "Presets", presetName)
	if err := os.MkdirAll(filepath.Dir(presetPath), 0o700); err != nil {
		return err
	}
	login := strconv.FormatInt(mustInt(j.Login), 10)
	if login == "0" {
		return fmt.Errorf("login akun %d harus berupa angka positif", j.AccountID)
	}
	copyAfter, err := w.copyAfter(j.AccountID)
	if err != nil {
		return err
	}
	if err := w.feed.Register(j.AccountID, login, j.Server, token, copyAfter); err != nil {
		return err
	}
	values := []string{
		"ServerURL=" + "http://" + w.cfg.FeedAddress,
		"AccountToken=" + token,
		"PollIntervalMs=250||250||100||5000||N",
		"MagicNumber=880001||880001||1||880001||N",
		"VolumeMultiplier=1.0||1.0||0.1||10.0||N",
		"MaxEntryDeviationPercent=0.0||0.0||0.1||10.0||N",
		"MaxSlippagePoints=20||20||1||100||N",
		"SymbolSuffix=",
		"SymbolMapCsv=XAUUSD=" + j.FollowerSymbol,
		"ManagedMode=true||true||false||true||N",
		"ManagedFileBridge=true||true||false||true||N",
		"BridgeChannel=" + strconv.FormatInt(j.AccountID, 10),
		"LotMode=" + j.ModeLot,
		fmt.Sprintf("FixedLot=%.8f||%.8f||0.01||100.0||N", j.FixedLot, j.FixedLot),
		fmt.Sprintf("RatioLot=%.8f||%.8f||0.01||100.0||N", j.RatioLot, j.RatioLot),
		"MasterBalance=10000.0||10000.0||100.0||1000000.0||N",
		fmt.Sprintf("MaxLotPerOrder=%.8f||%.8f||0.01||100.0||N", j.MaxLotPerOrder, j.MaxLotPerOrder),
		fmt.Sprintf("MaxLotTotal=%.8f||%.8f||0.01||1000.0||N", j.MaxLotTotal, j.MaxLotTotal),
		fmt.Sprintf("MaxOpenPositions=%d||%d||1||1000||N", j.MaxOpenPositions, j.MaxOpenPositions),
		"ReverseDirection=false||false||false||true||N",
	}
	if strings.TrimSpace(j.FollowerSymbol) == "" || strings.ContainsAny(j.FollowerSymbol, "\r\n,=") {
		return fmt.Errorf("simbol follower akun %d tidak valid untuk input EA", j.AccountID)
	}
	if err := writeAtomic(presetPath, []byte(strings.Join(values, "\r\n")+"\r\n")); err != nil {
		return fmt.Errorf("preset EA akun %d tidak dapat ditulis: %w", j.AccountID, err)
	}
	return nil
}

func (w *worker) copyAfter(accountID int64) (int64, error) {
	path := filepath.Join(w.cfg.runtimeDir, "states", fmt.Sprintf("%d.copy-after", accountID))
	if raw, err := os.ReadFile(path); err == nil {
		value, parseErr := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if parseErr != nil || value <= 0 {
			return 0, fmt.Errorf("copy-after akun %d tidak valid", accountID)
		}
		return value, nil
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	value := time.Now().Unix()
	if err := writeAtomic(path, []byte(strconv.FormatInt(value, 10))); err != nil {
		return 0, err
	}
	return value, nil
}

func writeManagedStartup(path string, j job, name, startupServer string) error {
	server, password := startupServer, j.BrokerPassword
	for _, value := range []string{server, password} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("kredensial terminal akun %d mengandung karakter baris baru", j.AccountID)
		}
	}
	content := fmt.Sprintf("[Common]\r\nLogin=%s\r\nServer=%s\r\nPassword=%s\r\nKeepPrivate=1\r\nNewsEnable=0\r\n\r\n[Experts]\r\nEnabled=1\r\nAllowLiveTrading=1\r\nAllowDllImport=0\r\nAccount=0\r\nProfile=0\r\n\r\n[StartUp]\r\nExpert=MT5FollowerCopyEA\r\nExpertParameters=%s.set\r\nSymbol=EURUSD\r\nPeriod=M1\r\n", j.Login, server, password, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("startup config MT5 akun %d tidak dapat ditulis: %w", j.AccountID, err)
	}
	return nil
}

func (w *worker) removeStartupConfigAfterLaunch(c *child) {
	// MT5 reads its custom startup INI during launch. The password is removed
	// shortly afterwards and is never placed in the terminal command line or EA preset.
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		if c.startupINI != "" {
			if err := os.Remove(c.startupINI); err != nil && !os.IsNotExist(err) {
				log.Printf("startup config akun %d tidak dapat dibersihkan: %v", c.accountID, err)
			}
		}
	}
}

func newFeedToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
