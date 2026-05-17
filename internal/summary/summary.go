package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/go-pdf/fpdf"
	"github.com/joho/godotenv"
)

func main() {
	// 1. Laad .env bestand
	if err := godotenv.Load(); err != nil {
		log.Println("Geen .env bestand gevonden, we gebruiken environment variables.")
	}

	// 2. Setup Elasticsearch Client
	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		Username:  os.Getenv("CONTROLROOM_ES_USER"),
		Password:  os.Getenv("CONTROLROOM_ES_PASS"),
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Fout bij aanmaken ES client: %s", err)
	}

	// Test connectie
	res, err := es.Info()
	if err != nil || res.IsError() {
		log.Fatalf("Kan niet verbinden met ES: %s", err)
	}
	res.Body.Close()
	log.Println("✅ Verbonden met Elasticsearch!")

	// 3. Bouw de PDF
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	// Titel
	pdf.SetFont("Arial", "B", 20)
	pdf.CellFormat(190, 10, "Daily Controlroom Performance Report", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "I", 10)
	pdf.CellFormat(190, 10, fmt.Sprintf("Gegenereerd op: %s", time.Now().Format("02-01-2006 15:04")), "", 1, "C", false, 0, "")
	pdf.Ln(10)

	// --- SECTIE 1: USERS ---
	generateUsersSection(pdf, es)

	// --- SECTIE 1b: COMPANIES ---
	generateCompaniesSection(pdf, es)

	// --- SECTIE 2: STATUSCHECKS (Averages) ---
	generateStatusSection(pdf, es)

	// --- SECTIE 3: CRITICAL LOGS & WATCHDOG ---
	generateLogsSection(pdf, es)

	// --- SECTIE 4: HEARTBEATS & UPTIME INDICATIE ---
	generateHeartbeatSection(pdf, es)

	// 4. Opslaan en Openen
	fileName := fmt.Sprintf("Report_%s.pdf", time.Now().Format("20060102"))
	err = pdf.OutputFileAndClose(fileName)
	if err != nil {
		log.Fatalf("Fout bij opslaan PDF: %s", err)
	}

	log.Printf("✅ PDF succesvol opgeslagen als %s", fileName)
	openPDF(fileName)
}

// --- HELPER FUNCTIES VOOR ELASTICSEARCH EN PDF ---

func generateUsersSection(pdf *fpdf.Fpdf, es *elasticsearch.Client) {
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "1. Users & Inschrijvingen (Laatste 24u)")
	pdf.Ln(10)

	// Query: Aantal users per Role
	query := `{
		"size": 0,
		"query": { "range": { "indexed": { "gte": "now-24h" } } },
		"aggs": { "roles": { "terms": { "field": "Role.keyword" } } }
	}`

	var result map[string]any
	runQuery(es, "users", query, &result)

	pdf.SetFont("Arial", "", 12)
	aggregations, _ := result["aggregations"].(map[string]any)
	roles, _ := aggregations["roles"].(map[string]any)
	buckets, _ := roles["buckets"].([]any)

	total := 0
	for _, b := range buckets {
		bucket := b.(map[string]any)
		count := int(bucket["doc_count"].(float64))
		total += count
		pdf.CellFormat(190, 8, fmt.Sprintf("- Rol: %s | Aantal: %d", bucket["key"], count), "", 1, "L", false, 0, "")
	}
	pdf.SetFont("Arial", "I", 11)
	pdf.CellFormat(190, 8, fmt.Sprintf("Totaal aangemaakte users vandaag: %d", total), "", 1, "L", false, 0, "")
	pdf.Ln(5)
}

func generateStatusSection(pdf *fpdf.Fpdf, es *elasticsearch.Client) {
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "2. Systeem Gezondheid (Averages laatste 24u)")
	pdf.Ln(10)

	// Query: Gemiddelde CPU, Mem en Disk per Service
	query := `{
		"size": 0,
		"query": { "range": { "timestamp": { "gte": "now-24h" } } },
		"aggs": {
			"services": {
				"terms": { "field": "serviceId.keyword" },
				"aggs": {
					"avg_cpu": { "avg": { "field": "systemLoad.cpu" } },
					"avg_mem": { "avg": { "field": "systemLoad.memory" } },
					"avg_disk": { "avg": { "field": "systemLoad.disk" } }
				}
			}
		}
	}`

	var result map[string]any
	runQuery(es, "statuscheck", query, &result) // Pas indexnaam aan indien nodig

	pdf.SetFont("Arial", "B", 10)
	// Tabel Header
	pdf.CellFormat(50, 8, "Service", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Avg CPU (%)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Avg Memory (%)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Avg Disk (%)", "1", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	aggregations, _ := result["aggregations"].(map[string]any)
	services, _ := aggregations["services"].(map[string]any)
	buckets, _ := services["buckets"].([]any)

	for _, b := range buckets {
		bucket := b.(map[string]any)
		svc := bucket["key"].(string)

		// Helper functie om veilig float64 uit geneste map te halen
		getValue := func(aggName string) float64 {
			if agg, ok := bucket[aggName].(map[string]any); ok && agg["value"] != nil {
				return agg["value"].(float64) * 100 // x100 voor percentage weergave
			}
			return 0.0
		}

		pdf.CellFormat(50, 8, svc, "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 8, fmt.Sprintf("%.2f%%", getValue("avg_cpu")), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, fmt.Sprintf("%.2f%%", getValue("avg_mem")), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, fmt.Sprintf("%.2f%%", getValue("avg_disk")), "1", 1, "C", false, 0, "")
	}
	pdf.Ln(10)
}

func generateLogsSection(pdf *fpdf.Fpdf, es *elasticsearch.Client) {
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "3. Incidenten & Fouten (Laatste 24u)")
	pdf.Ln(10)

	// Query: Haal de laatste 10 ERROR, FATAL, of PANIC logs op
	query := `{
		"size": 10,
		"sort": [ { "timestamp": { "order": "desc" } } ],
		"query": {
			"bool": {
				"must": [
					{ "range": { "timestamp": { "gte": "now-24h" } } },
					{ "terms": { "level.keyword": ["ERROR", "FATAL", "PANIC", "CRITICAL"] } }
				]
			}
		}
	}`

	var result map[string]any
	runQuery(es, "controlroom-logs", query, &result) // Pas indexnaam aan als jullie logs elders staan

	hitsMap, _ := result["hits"].(map[string]any)
	hits, _ := hitsMap["hits"].([]any)

	if len(hits) == 0 {
		pdf.SetFont("Arial", "I", 11)
		pdf.Cell(40, 10, "Geen kritieke fouten of crashes gedetecteerd vandaag. Goed werk!")
		return
	}

	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(30, 8, "Tijd", "1", 0, "C", false, 0, "")
	pdf.CellFormat(20, 8, "Level", "1", 0, "C", false, 0, "")
	pdf.CellFormat(30, 8, "Service", "1", 0, "C", false, 0, "")
	pdf.CellFormat(110, 8, "Foutmelding", "1", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 8)
	for _, h := range hits {
		hit := h.(map[string]any)
		source := hit["_source"].(map[string]any)

		// Check veldnamen in jullie mapping!
		ts := "Onbekend"
		if val, ok := source["timestamp"].(string); ok && len(val) > 16 {
			ts = val[:16] // Korte timestamp
		}
		lvl, _ := source["level"].(string)
		svc, _ := source["service"].(string)
		msg, _ := source["data"].(string)

		// Max lengte voor de message zodat hij in de tabel past
		if len(msg) > 75 {
			msg = msg[:72] + "..."
		}

		pdf.CellFormat(30, 8, ts, "1", 0, "C", false, 0, "")
		pdf.CellFormat(20, 8, lvl, "1", 0, "C", false, 0, "")
		pdf.CellFormat(30, 8, svc, "1", 0, "C", false, 0, "")
		pdf.CellFormat(110, 8, msg, "1", 1, "L", false, 0, "")
	}
}

func generateCompaniesSection(pdf *fpdf.Fpdf, es *elasticsearch.Client) {
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "1b. Companies (Laatste 24u)")
	pdf.Ln(10)

	query := `{
		"size": 50,
		"query": { "range": { "indexed": { "gte": "now-24h" } } }
	}`

	var result map[string]any
	runQuery(es, "companies", query, &result) // Pas aan naar jullie echte company index

	pdf.SetFont("Arial", "", 11)
	hitsMap, _ := result["hits"].(map[string]any)
	hits, _ := hitsMap["hits"].([]any)

	if len(hits) == 0 {
		pdf.CellFormat(190, 8, "- Geen nieuwe companies geregistreerd vandaag.", "", 1, "L", false, 0, "")
	} else {
		for _, h := range hits {
			hit := h.(map[string]any)
			source := hit["_source"].(map[string]any)
			name, _ := source["name"].(string) // Zorg dat dit overeenkomt met jullie DTO!
			pdf.CellFormat(190, 8, fmt.Sprintf("- Bedrijf aangemaakt: %s", name), "", 1, "L", false, 0, "")
		}
	}
	pdf.Ln(5)
}

func generateHeartbeatSection(pdf *fpdf.Fpdf, es *elasticsearch.Client) {
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "4. Heartbeats & Uptime Indicatie (Laatste 24u)")
	pdf.Ln(10)

	// Simpele aggregatie: Hoeveel heartbeats per service? (Verwachting: ~2880 per dag bij 1 per 30s)
	query := `{
		"size": 0,
		"query": { "range": { "timestamp": { "gte": "now-24h" } } },
		"aggs": { "services": { "terms": { "field": "service_id.keyword" } } }
	}`

	var result map[string]any
	runQuery(es, "heartbeats", query, &result)

	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(190, 8, "Aantal ontvangen heartbeats per service (Verwacht: ~2880/dag):", "", 1, "L", false, 0, "")

	aggregations, _ := result["aggregations"].(map[string]any)
	if services, ok := aggregations["services"].(map[string]any); ok {
		buckets, _ := services["buckets"].([]any)
		for _, b := range buckets {
			bucket := b.(map[string]any)
			count := int(bucket["doc_count"].(float64))
			pdf.CellFormat(190, 8, fmt.Sprintf("- %s: %d heartbeats", bucket["key"], count), "", 1, "L", false, 0, "")
		}
	}
	pdf.Ln(5)
}

// Helper om Elasticsearch queries uit te voeren
func runQuery(es *elasticsearch.Client, index string, query string, result *map[string]any) {
	res, err := es.Search(
		es.Search.WithIndex(index),
		es.Search.WithBody(bytes.NewReader([]byte(query))),
	)
	if err != nil {
		log.Printf("Netwerkfout bij query op index %s: %v", index, err)
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		// Dit print de daadwerkelijke ES error (bijv. 404 index_not_found_exception)
		log.Printf("⚠️ Elasticsearch error op index '%s': %s", index, res.String())
		return
	}

	json.NewDecoder(res.Body).Decode(result)
}

// Helper om de PDF lokaal direct te openen (Cross-platform)
func openPDF(fileName string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", fileName)
	case "darwin": // macOS
		cmd = exec.Command("open", fileName)
	default: // Linux
		cmd = exec.Command("xdg-open", fileName)
	}
	cmd.Start()
}
