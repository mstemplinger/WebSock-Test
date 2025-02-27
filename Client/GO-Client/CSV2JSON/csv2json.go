package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

type MetaData struct {
	ContentType string `json:"ContentType"`
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

type Content struct {
	TableName    string              `json:"TableName"`
	Consts       []Const             `json:"Consts"`
	FieldMapping []FieldMapping      `json:"FieldMappings"`
	Data         []map[string]string `json:"Data"`
}

type ExportData struct {
	MetaData MetaData `json:"MetaData"`
	Content  Content  `json:"Content"`
}

func main() {
	infile := flag.String("infile", "", "Pfad zur CSV-Datei")
	outfile := flag.String("outfile", "", "Pfad zur JSON-Ausgabedatei")
	dbtable := flag.String("dbtable", "", "Datenbanktabelle für den Import")
	flag.Parse()

	if *infile == "" || *outfile == "" || *dbtable == "" {
		fmt.Println("❌ Fehler: infile, outfile und dbtable müssen angegeben werden")
		os.Exit(1)
	}

	file, err := os.Open(*infile)
	if err != nil {
		fmt.Println("❌ Fehler beim Öffnen der Datei:", err)
		os.Exit(1)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		fmt.Println("❌ Fehler beim Lesen der CSV-Datei:", err)
		os.Exit(1)
	}

	if len(rows) < 2 {
		fmt.Println("❌ Fehler: CSV enthält keine Daten")
		os.Exit(1)
	}

	headers := rows[0]
	var data []map[string]string
	for _, row := range rows[1:] {
		record := make(map[string]string)
		for i, value := range row {
			if i < len(headers) {
				record[headers[i]] = value
			}
		}
		data = append(data, record)
	}

	var fieldMappings []FieldMapping
	for _, header := range headers {
		fieldMappings = append(fieldMappings, FieldMapping{
			TargetField:  header,
			Expression:   fmt.Sprintf("{%s}", header),
			IsIdentifier: false,
			ImportField:  true,
		})
	}

	export := ExportData{
		MetaData: MetaData{
			ContentType: "db-import",
			Name:        "CSV Import",
			Description: "Dynamischer Import von CSV-Daten",
			Version:     "1.0",
			Creator:     "ondeso",
			Vendor:      "ondeso GmbH",
			Preview:     "",
			Schema:      "",
		},
		Content: Content{
			TableName:    *dbtable,
			Consts:       []Const{{Identifier: "CaptureDate", Value: time.Now().Format(time.RFC3339)}},
			FieldMapping: fieldMappings,
			Data:         data,
		},
	}

	jsonData, err := json.MarshalIndent(export, "", "    ")
	if err != nil {
		fmt.Println("❌ Fehler beim Generieren von JSON:", err)
		os.Exit(1)
	}

	err = os.WriteFile(*outfile, jsonData, 0644)
	if err != nil {
		fmt.Println("❌ Fehler beim Speichern der JSON-Datei:", err)
		os.Exit(1)
	}

	fmt.Println("✅ JSON erfolgreich erstellt:", *outfile)
}
