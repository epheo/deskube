package googlestorage

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// GoogleStorage represents the structure of the storage information from Google Cloud Storage API

func GoogleStorageDownload(project string, arch string, file string) string {
	dir := "out/bin"

	// Ensure the directory exists
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatalf("Failed to create folder: %v", err)
	}

	// Get the latest version
	version := getLatest(project)

	url := fmt.Sprintf("https://storage.googleapis.com/%s/release/%s/bin/%s/%s", project, version, arch, file)

	tokens := strings.Split(url, "/")
	fileName := dir + "/" + tokens[len(tokens)-1]
	if fileName == "" {
		log.Fatalf("could not extract file name from URL")
		return ""
	}
	response, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to download file: %v", err)
		return ""
	}
	defer response.Body.Close()

	out, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
		return ""
	}
	defer out.Close()

	_, err = io.Copy(out, response.Body)
	if err != nil {
		log.Fatalf("Failed to copy file: %v", err)
		return ""
	}

	log.Println("Downloaded", fileName)
	return fileName
}

func getLatest(project string) string {

	url := fmt.Sprintf("https://storage.googleapis.com/%s/release/stable.txt", project)

	// Make an HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to fetch version: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	// Convert body to string and print the version
	version := string(body)
	log.Printf("Kubernetes stable version: %s\n", version)
	return version
}
