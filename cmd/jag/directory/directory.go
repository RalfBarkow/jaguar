// Copyright (C) 2021 Toitware ApS. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package directory

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

const (
	// UserConfigPathEnv if set, will load the user config from that path.
	UserConfigPathEnv    = "JAG_USER_CONFIG_PATH"
	DeviceConfigPathEnv  = "JAG_DEVICE_CONFIG_PATH"
	SnapshotCachePathEnv = "JAG_SNAPSHOT_CACHE_PATH"
	configFile           = ".jaguar"

	// ToitPathEnv: Path to the Toit SDK build.
	ToitRepoPathEnv = "JAG_TOIT_REPO_PATH"
	// WifiSSIDEnv if set will use this wifi ssid.
	WifiSSIDEnv = "JAG_WIFI_SSID"
	// WifiPasswordEnv if set will use this wifi password.
	WifiPasswordEnv = "JAG_WIFI_PASSWORD"
)

// Hackishly set by main.go.
var IsReleaseBuild = false

func GetUserConfigPath() (string, error) {
	if path, ok := os.LookupEnv(UserConfigPathEnv); ok {
		return path, nil
	}

	homedir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homedir, ".config", "jaguar", "config.yaml"), nil
}

func GetDeviceConfigPath() (string, error) {
	if path, ok := os.LookupEnv(DeviceConfigPathEnv); ok {
		return path, nil
	}

	homedir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homedir, ".config", "jaguar", "device.yaml"), nil
}

func GetSnapshotsCachePath() (string, error) {
	path, ok := os.LookupEnv(SnapshotCachePathEnv)
	if ok {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return ensureDirectory(filepath.Join(home, ".cache", "jaguar", "snapshots"), nil)
}

func getRepoPath() (string, bool) {
	if IsReleaseBuild {
		return "", false
	}
	return os.LookupEnv(ToitRepoPathEnv)
}

func GetSDKPath() (string, error) {
	repoPath, ok := getRepoPath()
	if ok {
		return filepath.Join(repoPath, "build", "host", "sdk"), nil
	}
	sdkCachePath, err := GetSDKCachePath()
	if err != nil {
		return "", err
	}
	if stat, err := os.Stat(sdkCachePath); err != nil || !stat.IsDir() {
		return "", fmt.Errorf("no SDK found in '%s'.\nYou must setup the esp32 image using 'jag setup'", sdkCachePath)
	}
	return sdkCachePath, nil
}

func GetSDKCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "jaguar", "sdk"), nil
}

func GetESP32ImageCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "jaguar", "image"), nil
}

func GetESP32CachePath() (string, error) {
	imagePath, err := GetESP32ImageCachePath()
	if err != nil {
		return "", err
	}
	if stat, err := os.Stat(imagePath); err != nil || !stat.IsDir() {
		return "", fmt.Errorf("the path '%s' did not hold the esp32 image.\nYou must setup the esp32 image using 'jag setup'", imagePath)
	}
	return imagePath, nil
}

func GetToitToolchainPath() (string, error) {
	repoPath, ok := getRepoPath()
	if ok {
		return filepath.Join(repoPath, "toolchains", "esp32"), nil
	}

	return GetESP32CachePath()
}

func GetESP32ImagePath() (string, error) {
	repoPath, ok := getRepoPath()
	if ok {
		return filepath.Join(repoPath, "build", "esp32"), nil
	}

	return GetESP32CachePath()
}

func getImageSnapshotPath(name string) (string, error) {
	_, ok := getRepoPath()
	if ok {
		// We assume that the jag executable is inside the build directory of
		// the Jaguar repository.
		execPath, err := os.Executable()
		if err != nil {
			return "", err
		}
		dir := path.Dir(execPath)
		return filepath.Join(dir, "image", name), nil
	}

	imagePath, err := GetESP32ImagePath()
	if err != nil {
		return "", err
	}
	snapshotPath := filepath.Join(imagePath, name)

	if stat, err := os.Stat(snapshotPath); err != nil || stat.IsDir() {
		return "", fmt.Errorf("the path '%s' did not hold the snapshot file.\nYou must setup the Jaguar snapshot using 'jag setup'", snapshotPath)
	}
	return snapshotPath, nil
}

func GetJaguarSnapshotPath() (string, error) {
	return getImageSnapshotPath("jaguar.snapshot")
}

func GetSystemSnapshotPath() (string, error) {
	return getImageSnapshotPath("system.snapshot")
}

func GetEsptoolCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "jaguar", Executable("esptool")), nil
}

func GetEsptoolPath() (string, error) {
	repoPath, ok := getRepoPath()
	if ok {
		return filepath.Join(repoPath, "third_party", "esp-idf", "components", "esptool_py", "esptool", "esptool.py"), nil
	}

	cachePath, err := GetEsptoolCachePath()
	if err != nil {
		return "", err
	}

	if stat, err := os.Stat(cachePath); err != nil || stat.IsDir() {
		return "", fmt.Errorf("the path '%s' did not hold the esptool.\nYou must setup the esptool using 'jag setup'", cachePath)
	}
	return cachePath, nil
}

func ensureDirectory(dir string, err error) (string, error) {
	if err != nil {
		return dir, err
	}
	return dir, os.MkdirAll(dir, 0755)
}

func GetUserConfig() (*viper.Viper, error) {
	path, err := GetUserConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get user config path: %w", err)
	}

	cfg := viper.New()
	cfg.SetConfigType("yaml")
	cfg.SetConfigFile(path)
	if _, err := os.Stat(path); err == nil {
		if err := cfg.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read user config: %w", err)
		}
	}
	return cfg, nil
}

func GetDeviceConfig() (*viper.Viper, error) {
	path, err := GetDeviceConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get device config path: %w", err)
	}

	cfg := viper.New()
	cfg.SetConfigType("yaml")
	cfg.SetConfigFile(path)
	if _, err := os.Stat(path); err == nil {
		if err := cfg.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read device config: %w", err)
		}
	}
	return cfg, nil
}

func WriteConfig(cfg *viper.Viper) error {
	file := cfg.ConfigFileUsed()
	dir := filepath.Dir(file)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	tmpFile := filepath.Join(filepath.Dir(file), ".config.tmp.yaml")
	if err := cfg.WriteConfigAs(tmpFile); err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	return os.Rename(tmpFile, file)
}

func Executable(str string) string {
	if runtime.GOOS == "windows" {
		return str + ".exe"
	}
	return str
}
