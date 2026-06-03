package auth

import (
	"os"
	"path/filepath"
)

const configDirName = ".1agents"
const tokenFileName = "access-token"

func TokenFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, configDirName, tokenFileName)
}

func ensureConfigDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, configDirName)
	return os.MkdirAll(configDir, 0755)
}

func SaveToken(token string) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}
	return os.WriteFile(TokenFilePath(), []byte(token), 0600)
}

func LoadToken() (string, error) {
	data, err := os.ReadFile(TokenFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func DeleteToken() error {
	err := os.Remove(TokenFilePath())
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func TokenExists() bool {
	_, err := os.Stat(TokenFilePath())
	return err == nil
}
