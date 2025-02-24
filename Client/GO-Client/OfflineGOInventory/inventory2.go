package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type MetaData struct {
	ContentType  string `json:"ContentType"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Version     string `json:"Version"`
	Creator     string `json:"Creator"`
	Vendor      string `json:"Vendor"`
	Preview     string `json:"Preview"`
	Schema      string `json:"Schema"`
}

type Const struct {
	Identifier string `json:"Identifier"`
	Value      string `json:"Value"`
}

type FieldMapping struct {
	TargetField  string `json:"TargetField"`
	Expression   string `json:"Expression"`
	IsIdentifier bool   `json:"IsIdentifier"`
	ImportField  bool   `json:"ImportField"`
}

type DataEntry struct {
	ID            string `json:"id"`
	TransactionID string `json:"transaction_id"`
	AssetID       string `json:"asset_id"`
	Username      string `json:"username"`
	Client        string `json:"client"`
	Usercount     string `json:"usercount"`
	Permissions   string `json:"permissions"`
	SID           string `json:"sid"`
	FullName      string `json:"full_name"`
	AccountStatus string `json:"account_status"`
	LastLogon     string `json:"last_logon"`
	Description   string `json:"description"`
}

type Content struct {
	TableName     string         `json:"TableName"`
	Consts        []Const        `json:"Consts"`
	FieldMappings []FieldMapping `json:"FieldMappings"`
	Data          []DataEntry    `json:"Data"`
}

type Inventory struct {
	MetaData MetaData `json:"MetaData"`
	Content  Content  `json:"Content"`
}

func getSystemInfo() []DataEntry {
	cmd := exec.Command("wmic", "useraccount", "get", "Name,SID")
	out, _ := cmd.Output()
	lines := strings.Split(string(out), "\n")
	var users []DataEntry
	transactionID := "58ad96ce-2427-451e-af43-2fe2f5730cd8"
	assetID := "EFC0819A-5184-4422-99A8-80FEB75FA64B"

	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			users = append(users, DataEntry{
				ID:            "uuid-generated-here",
				TransactionID: transactionID,
				AssetID:       assetID,
				Username:      fields[0],
				Client:        "MatinsSurfaceLS",
				Usercount:     fmt.Sprintf("%d", len(users)+1),
				Permissions:   "",
				SID:           fields[1],
				FullName:      "",
				AccountStatus: "Active",
				LastLogon:     "",
				Description:   "",
			})
		}
	}
	return users
}

func saveJSONToFile(jsonData []byte, filePath string) error {
	return os.WriteFile(filePath, jsonData, 0644)
}

func main() {
	var filePath string
	if len(os.Args) > 1 {
		filePath = os.Args[1]
	} else {
		workplaceDir := "workplace"
		offlineDir := filepath.Join(workplaceDir, "OfflineInventory")
		os.MkdirAll(offlineDir, os.ModePerm)
		fileName := fmt.Sprintf("%s_OfflineInventory.json", time.Now().Format("2006-01-02_15-04-05"))
		filePath = filepath.Join(offlineDir, fileName)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	users := getSystemInfo()

	inventory := Inventory{
		MetaData: MetaData{
			ContentType:  "db-import",
			Name:        "Windows System Inventory",
			Description: "Collect System Information",
			Version:     "1.0",
			Creator:     "ondeso",
			Vendor:      "ondeso GmbH",
			Preview:     "",
			Schema:      "",
		},
		Content: Content{
			TableName: "usr_client_users",
			Consts: []Const{{
				Identifier: "CaptureDate",
				Value:      now,
			}},
			FieldMappings: []FieldMapping{
				{"transaction_id", "{transaction_id}", true, true},
				{"asset_id", "{asset_id}", false, true},
				{"username", "{username}", false, true},
				{"client", "{client}", false, true},
				{"usercount", "{usercount}", false, true},
				{"permissions", "{permissions}", false, true},
				{"sid", "{sid}", false, true},
				{"full_name", "{full_name}", false, true},
				{"account_status", "{account_status}", false, true},
				{"last_logon", "{last_logon}", false, true},
				{"description", "{description}", false, true},
			},
			Data: users,
		},
	}

	jsonData, err := json.MarshalIndent(inventory, "", "    ")
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	if err := saveJSONToFile(jsonData, filePath); err != nil {
		fmt.Println("Error saving JSON to file:", err)
	} else {
		fmt.Println("Inventory saved to:", filePath)
	}
}
