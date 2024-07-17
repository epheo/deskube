package github

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

func GithubDownload(project string) error {

	dir := "out/bin"

	// Ensure the directory exists
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	url, contentType := getLatest(project)
	tokens := strings.Split(url, "/")
	fileName := dir + "/" + tokens[len(tokens)-1]
	if fileName == "" {
		return errors.New("could not extract file name from URL")
	}
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if contentType == "application/gzip" {
		// Handle gzip content
		return extractTarGz(response.Body, fileName)
	} else {
		// Handle other content types
		out, err := os.Create(fileName)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, response.Body)

		fmt.Println("Downloaded", fileName)
		return err
	}
}

func getLatest(project string) (url string, content_type string) {

	url = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", project)
	// Make an HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error fetching release information:", err)
		return "", ""
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return "", ""
	}

	// Parse the JSON response
	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		fmt.Println("Error parsing JSON response:", err)
		return "", ""
	}

	// Find the asset for linux-amd64
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "linux-amd64") {
			return asset.BrowserDownloadURL, asset.ContentType
		}
	}

	fmt.Println("No binary found for linux-amd64")
	return "", ""

}

// extractTarGz extracts a .tar.gz file from an io.Reader and writes the contents to the specified directory.
func extractTarGz(gzipStream io.Reader, dir string) error {
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return err
	}
	defer uncompressedStream.Close()

	tarReader := tar.NewReader(uncompressedStream)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Construct the path to extract to
		path := filepath.Join(dir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	fmt.Println("Extracted to", dir)
	return nil
}
