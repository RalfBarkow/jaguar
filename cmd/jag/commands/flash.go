// Copyright (C) 2021 Toitware ApS. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package commands

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/toitlang/jaguar/cmd/jag/directory"
)

// Returns position, size of the partitions in partitions.csv.
func getPartitions(toitToolchainPath string, positions map[string]int, sizes map[string]int) {
	// Load default partitions positions, that can be overridden by the
	// partitions.csv file.  Some of these (bootloader, partitions) are
	// commented out in the partitions.csv because they can't be changed.
	positions["bootloader"] = 0x1000
	positions["partitions"] = 0x8000
	positions["ota"] = 0xd000
	positions["ota_0"] = 0x10000
	sizes["bootloader"] = 0x7000
	sizes["partitions"] = 0x0c00
	COLUMN_NAME := 0
	// COLUMN_TYPE := 1
	COLUMN_SUBTYPE := 2
	COLUMN_POSITION := 3
	COLUMN_SIZE := 4

	file, err := os.Open(filepath.Join(toitToolchainPath, "partitions.csv"))
	if err != nil {
		panic("Could not find partitions.csv")
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		partitionType := ""
		line := scanner.Text()
		comment := strings.Index(line, "#")
		if comment != -1 {
			line = line[:comment]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			fields := strings.Split(line, ",")
			maxIndex := -1
			for index, field := range fields {
				maxIndex = index
				field = strings.TrimSpace(field)
				if index == COLUMN_NAME {
					partitionType = field
				} else if index == COLUMN_SUBTYPE {
					if field != "" {
						partitionType = field
					}
				} else if index == COLUMN_POSITION || index == COLUMN_SIZE {
					num, err := strconv.ParseInt(field, 0, 32)
					if err != nil || partitionType == "" {
						panic("Could not parse number in partitions.csv")
					} else {
						if index == COLUMN_POSITION {
							positions[partitionType] = int(num)
						} else {
							sizes[partitionType] = int(num)
						}
					}
				}
			}
			if maxIndex < COLUMN_SIZE {
				panic("Could not parse line in partitions.csv (missing fields)")
			}
		}
	}
}

func hex(num int) string {
	return fmt.Sprintf("0x%x", num)
}

func createZapBytesFile(sizes map[string]int, name string) (*os.File, error) {
	// Create a file with zap bytes (0xff) for clearing select partitions.
	zappedDataFile, err := os.CreateTemp("", fmt.Sprint("*.%sdata", name))
	if err != nil {
		return nil, err
	}

	if size, ok := sizes[name]; ok {
		_, err = zappedDataFile.Write(bytes.Repeat([]byte{0xff}, size))
		if err != nil {
			os.Remove(zappedDataFile.Name())
			return nil, err
		}
	} else {
		fmt.Printf("No size for %s partition, skipping\n", name)
	}
	zappedDataFile.Close()
	return zappedDataFile, nil
}

func FlashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flash",
		Short: "Flash an ESP32 with the Jaguar image",
		Long: "Flash an ESP32 with the Jaguar application image. The initial flashing is\n" +
			"done over a serial connection and it is used to give the ESP32 its initial\n" +
			"firmware and the necessary WiFi credentials.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			port, err := cmd.Flags().GetString("port")
			if err != nil {
				return err
			}
			if port, err = CheckPort(port); err != nil {
				return err
			}

			baud, err := cmd.Flags().GetUint("baud")
			if err != nil {
				return err
			}

			id := uuid.New()
			var name string
			if cmd.Flags().Changed("name") {
				name, err = cmd.Flags().GetString("name")
				if err != nil {
					return err
				}
			} else {
				name = GetRandomName(id[:])
			}

			wifiSSID, wifiPassword, err := getWifiCredentials(cmd)
			if err != nil {
				return err
			}

			esptoolPath, err := directory.GetEsptoolPath()
			if err != nil {
				return err
			}

			esp32BinPath, err := directory.GetESP32ImagePath()
			if err != nil {
				return err
			}

			toitToolchainPath, err := directory.GetToitToolchainPath()
			if err != nil {
				return err
			}

			binTmpFile, err := BuildFirmwareImage(ctx, id.String(), name, wifiSSID, wifiPassword)
			if err != nil {
				return err
			}
			defer os.Remove(binTmpFile.Name())

			positions := make(map[string]int)
			sizes := make(map[string]int)

			getPartitions(toitToolchainPath, positions, sizes)

			// Create a file with zap bytes (0xff) for clearing the OTA data partition.
			zappedOtaDataFile, err := createZapBytesFile(sizes, "ota")
			if err != nil {
				return err
			}
			defer os.Remove(zappedOtaDataFile.Name())

			// Create a file with zap bytes (0xff) for clearing the NVS data partition.
			zappedNvsDataFile, err := createZapBytesFile(sizes, "nvs")
			if err != nil {
				return err
			}
			defer os.Remove(zappedNvsDataFile.Name())

			flashArgs := []string{
				"--chip", "esp32", "--port", port, "--baud", strconv.Itoa(int(baud)), "--before", "default_reset", "--after", "hard_reset", "write_flash", "-z", "--flash_mode", "dio",
				"--flash_freq", "40m", "--flash_size", "detect",
				hex(positions["bootloader"]), filepath.Join(esp32BinPath, "bootloader", "bootloader.bin"),
				hex(positions["partitions"]), filepath.Join(esp32BinPath, "partitions.bin"),
				hex(positions["ota_0"]), binTmpFile.Name(),
			}
			if pos, ok := positions["ota"]; ok {
				// Force bootloader to boot from OTA 0.
				flashArgs = append(flashArgs, hex(pos), zappedOtaDataFile.Name())
			}
			if pos, ok := positions["nvs"]; ok {
				flashArgs = append(flashArgs, hex(pos), zappedNvsDataFile.Name())
			}

			fmt.Printf("Flashing device over serial on port '%s' ...\n", port)
			flashCmd := exec.CommandContext(ctx, esptoolPath, flashArgs...)
			flashCmd.Stderr = os.Stderr
			flashCmd.Stdout = os.Stdout
			return flashCmd.Run()
		},
	}

	cmd.Flags().StringP("port", "p", ConfiguredPort(), "serial port to flash via")
	cmd.Flags().Uint("baud", 921600, "baud rate used for the serial flashing")
	cmd.Flags().String("wifi-ssid", "", "default WiFi SSID")
	cmd.Flags().String("wifi-password", "", "default WiFi password")
	cmd.Flags().String("name", "", "name for the device, if not set a name will be auto generated")
	return cmd
}
