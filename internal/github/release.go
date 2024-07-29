package github

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/epheo/deskube/internal/system"
)

// GitHubRelease represents the structure of the release information from GitHub API
type GitHubRelease struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

func GithubDownload(project string, arch string, dir string) string {

	// Ensure the directory exists
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return ""
	}

	url, contentType := getLatest(project, arch)
	tokens := strings.Split(url, "/")
	fileName := dir + "/" + tokens[len(tokens)-1]
	if fileName == "" {
		log.Fatalf("could not extract file name from URL")
		return ""
	}
	response, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer response.Body.Close()

	if contentType == "application/gzip" {
		// Handle gzip content
		system.ExtractTarGz(response.Body, dir)
	} else {
		// Handle other content types
		out, err := os.Create(fileName)
		if err != nil {
			return ""
		}
		defer out.Close()

		_, _ = io.Copy(out, response.Body)

		log.Println("Downloaded", fileName)
		return ""
	}

	return dir
}

func getLatest(project string, arch string) (url string, content_type string) {

	url = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", project)
	// Make an HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error fetching release information:", err)
		return "", ""
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
		return "", ""
	}

	// Parse the JSON response
	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		log.Println("Error parsing JSON response:", err)
		return "", ""
	}

	// Find the asset for linux-amd64
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, arch) {
			return asset.BrowserDownloadURL, asset.ContentType
		}
	}

	log.Fatalf("No binary found for %s", arch)
	return "", ""

}
