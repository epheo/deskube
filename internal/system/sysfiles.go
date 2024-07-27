package system

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"text/template"
)

// moveFile moves a file from src to dst and can overwrite the destination file.
func MoveFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}

	return os.Remove(src)
}

// CopyFile copy a file from src to dst and can overwrite the destination file.
func CopyFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return nil
}

func FindFiles(root string, targets []string) ([]string, error) {
	var foundFiles []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		for _, target := range targets {
			if info.Name() == target {
				foundFiles = append(foundFiles, path)
			}
		}
		return nil
	})

	return foundFiles, err
}

func InstallBin(binFiles []string, destinationDir string) {

	for _, file := range binFiles {

		destFile := filepath.Join(destinationDir, filepath.Base(file))
		if err := CopyFile(file, destFile); err != nil {
			log.Printf("Error moving file %s: %s\n", file, err)
			continue
		}

		// 3. Change the file permissions to make it executable
		if err := os.Chmod(destFile, 0755); err != nil {
			log.Printf("Error changing permissions of file %s: %s\n", destFile, err)
		} else {
			log.Printf("Successfully moved and made executable: %s\n", destFile)
		}
	}

}

func TemplateFile(templatePath string, destinationPath string, data interface{}) error {
	// Template file
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}
	// Create a new file to write the output
	outputFile, err := os.Create(destinationPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %s", err)
	}
	defer outputFile.Close()
	// Execute the template with the data
	err = tmpl.Execute(outputFile, data)
	if err != nil {
		log.Println("Error executing template:", err)
		return err
	}
	log.Printf("Template written to %s successfully", outputFile.Name())

	return nil
}
