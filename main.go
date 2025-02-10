package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const baseURL = "https://ecrm.taxservice.am/taxsystem-rs-vcr/api/v1.0/"

var endpoints = map[string]string{
	"checkConnection":        "checkConnection",
	"activate":               "activate",
	"configureDepartments":   "configureDepartments",
	"getGoodList":            "getGoodList",
	"print":                  "print",
	"printCopy":              "printCopy",
	"getReturnedReceiptInfo": "getReturnedReceiptInfo",
	"printReturnReceipt":     "printReturnReceipt",
	"uploadCertificate":      "uploadCertificate",
}

type Crn struct {
	Value string `json:"crn"`
}

func findCertificateFiles(crn string) (string, string, error) {
	certFileCrt := filepath.Join("certs", crn+"*.crt")
	keyFileKey := filepath.Join("certs", crn+"*.key")

	certFiles, err := filepath.Glob(certFileCrt)
	if err != nil {
		return "", "", err
	}

	keyFiles, err := filepath.Glob(keyFileKey)
	if err != nil {
		return "", "", err
	}

	if len(certFiles) != 1 || len(keyFiles) != 1 {
		return "", "", fmt.Errorf("certificate or key file not found or multiple files found for crn: %s", crn)
	}

	return certFiles[0], keyFiles[0], nil
}

func checkCertificates(crn string) (string, string, error) {
	certPath, keyPath, err := findCertificateFiles(crn)
	if err != nil {
		return "", "", err
	}

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("certificate file not found: %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("key file not found: %s", keyPath)
	}

	log.Println("Certificates found successfully")

	uploadURL := fmt.Sprintf("%s%s", baseURL, "uploadCertificate")

	virtualHdm := exec.Command(
		"curl", "-X", "POST",
		uploadURL,
		"-H", "Content-Type: multipart/form-data",
		"-F", fmt.Sprintf("certificate=@%s", certPath),
		"-F", fmt.Sprintf("key=@%s", keyPath),
		"-F", fmt.Sprintf("crn=%s", crn),
	)

	output, err := virtualHdm.Output()
	if err != nil {
		return "", "", fmt.Errorf("error uploading certificates: %v\nResponse:\n%s", err, output)
	}

	log.Println("Certificates uploaded successfully")

	return certPath, keyPath, nil
}

func runCurlCommand(endpointKey string, jsonData map[string]any) (string, error) {
	endpoint, exists := endpoints[endpointKey]
	if !exists {
		return "", fmt.Errorf("unknown endpoint %s", endpointKey)
	}

	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return "", fmt.Errorf("JSON encoding error: %v", err)
	}

	crnInterface, ok := jsonData["crn"]
	if !ok {
		return "", fmt.Errorf("field 'crn' is missing")
	}

	crn, ok := crnInterface.(string)
	if !ok {
		return "", fmt.Errorf("field 'crn' must be a string")
	}

	_, err = strconv.Atoi(crn)
	if err != nil {
		return "", fmt.Errorf("field 'crn' must contain only numbers")
	}

	certPath, keyPath, err := checkCertificates(crn)
	if err != nil {
		return "", err
	}

	virtualHdm := exec.Command(
		"curl", "-X", "POST",
		baseURL+endpoint,
		"-H", "Content-Type: application/json",
		"--cert", certPath,
		"--key", keyPath,
		"-d", string(jsonBytes),
	)

	output, err := virtualHdm.Output()
	if err != nil {
		return "", fmt.Errorf("error in %s: %v\nResponse:\n%s", endpointKey, err, output)
	}

	fmt.Printf("%s result: %s\n", endpointKey, string(output))
	return string(output), nil
}

func handleRequest(w http.ResponseWriter, r *http.Request, endpointKey string) {
	if r.Method != http.MethodPost {
		currentTime := time.Now().Format("2006-01-02T15:04:05.000") + "+0000"
		errorResponse := map[string]interface{}{
			"timestamp": currentTime,
			"status":    405,
			"error":     "Method Not Allowed",
			"message":   "Request method 'GET' not supported",
			"path":      r.URL.Path,
		}

		jsonResponse, err := json.Marshal(errorResponse)
		if err != nil {
			http.Error(w, "Error marshaling JSON", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var jsonData map[string]any
	err = json.Unmarshal(body, &jsonData)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	crnInterface, ok := jsonData["crn"]
	if !ok {
		http.Error(w, "Field 'crn' is missing", http.StatusBadRequest)
		return
	}

	crn, ok := crnInterface.(string)
	if !ok {
		http.Error(w, "Field 'crn' must be a string", http.StatusBadRequest)
		return
	}

	_, err = strconv.Atoi(crn)
	if err != nil {
		http.Error(w, "Field 'crn' must contain only numbers", http.StatusBadRequest)
		return
	}

	response, err := runCurlCommand(endpointKey, jsonData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error processing request: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

func main() {
	flag.Parse()

	http.HandleFunc("/checkConnection", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "checkConnection")
	})
	http.HandleFunc("/activate", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "activate")
	})
	http.HandleFunc("/configureDepartments", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "configureDepartments")
	})
	http.HandleFunc("/getGoodList", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "getGoodList")
	})
	http.HandleFunc("/print", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "print")
	})
	http.HandleFunc("/printCopy", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "printCopy")
	})
	http.HandleFunc("/getReturnedReceiptInfo", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "getReturnedReceiptInfo")
	})
	http.HandleFunc("/printReturnReceipt", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "printReturnReceipt")
	})
	http.HandleFunc("/uploadCertificate", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, "uploadCertificate")
	})

	log.Println("Server is running :8019")
	log.Fatal(http.ListenAndServe(":8019", nil))
}

