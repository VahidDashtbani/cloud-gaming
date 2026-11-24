package main

import (
	"archive/zip"
	"bytes"
	"cloud-gaming/pkg/protocol"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
)

func DefaultGamesDir() string {
	if envDir := os.Getenv("GAMES_DIR"); envDir != "" {
		return envDir
	}
	if _, err := os.Stat(`C:\`); err == nil {
		return `C:\CloudGames`
	}
	return "./cloud-games"
}

func HandleGameTransfer(p protocol.GameTransferPayload) protocol.GameAckPayload {
	raw, err := base64.StdEncoding.DecodeString(p.DataB64)
	if err != nil {
		return protocol.GameAckPayload{GameName: p.GameName, Success: false, Error: "خطا در دیکد کردن فایل: " + err.Error()}
	}

	targetDir := filepath.Join(DefaultGamesDir(), p.GameName)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return protocol.GameAckPayload{GameName: p.GameName, Success: false, Error: "خطا در ساخت پوشه بازی: " + err.Error()}
	}

	outFile := filepath.Join(targetDir, filepath.Base(p.FileName))
	if err := os.WriteFile(outFile, raw, 0644); err != nil {
		return protocol.GameAckPayload{GameName: p.GameName, Success: false, Error: "خطا در ذخیره فایل بازی: " + err.Error()}
	}

	if p.Unzip {
		if err := unzipFile(outFile, targetDir); err != nil {
			return protocol.GameAckPayload{GameName: p.GameName, Success: false, Error: "خطا در اکسترکت ZIP: " + err.Error()}
		}
		_ = os.Remove(outFile)
	}

	return protocol.GameAckPayload{
		GameName: p.GameName,
		Success:  true,
		DestPath: targetDir,
	}
}

func unzipFile(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			_ = out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func BuildTransferPayload(gameName, filePath string, unzip bool) (protocol.GameTransferPayload, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return protocol.GameTransferPayload{}, err
	}
	var buf bytes.Buffer
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	_, _ = enc.Write(data)
	_ = enc.Close()

	return protocol.GameTransferPayload{
		GameName: gameName,
		FileName: filepath.Base(filePath),
		DataB64:  buf.String(),
		Unzip:    unzip,
	}, nil
}
