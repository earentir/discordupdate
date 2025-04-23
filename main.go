package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

var appversion = "0.0.1"

// progressWriter wraps an io.Writer to print progress information.
type progressWriter struct {
	total   int64
	written int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	// Print a simple progress update (you can enhance this as needed).
	fmt.Printf("\rDownloading... %d bytes", pw.written)
	return n, nil
}

// downloadFile downloads the file from the given URL to the specified filepath.
func downloadFile(url, filepath string) error {
	fmt.Println("Step 1: Starting download...")

	// Create file
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	// Create our progress writer
	pw := &progressWriter{}
	// Write the body to file and update progress.
	_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
	if err != nil {
		return fmt.Errorf("error while downloading: %w", err)
	}

	fmt.Println("\nDownload complete.")
	return nil
}

// removeDirIfExists removes the directory if it exists.
func removeDirIfExists(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		fmt.Println("Removing existing Discord folder at", path)
		return os.RemoveAll(path)
	}
	return nil
}

// extractTarGz extracts a tar.gz file to the destination directory.
func extractTarGz(srcFile, destDir string) error {
	fmt.Println("Step 2: Starting extraction...")
	file, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("failed to open tar.gz: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tarReader := tar.NewReader(gzr)

	// Extract each file from tar archive.
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("error during extraction: %w", err)
		}

		// Use the header to determine the file path.
		name := header.Name

		// In many archives, files are contained within a top-level folder.
		// We extract them directly to destDir (later we can rename if needed).
		target := filepath.Join(destDir, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("error creating directory: %w", err)
			}
		case tar.TypeReg:
			// Ensure directory exists.
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("error creating parent directory: %w", err)
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("error creating file: %w", err)
			}
			// Optionally, you can show progress here for each file.
			_, err = io.Copy(outFile, tarReader)
			outFile.Close()
			if err != nil {
				return fmt.Errorf("error writing file: %w", err)
			}
			fmt.Println("Extracted file:", target)
		default:
			fmt.Printf("Skipping unsupported file type: %s\n", name)
		}
	}

	fmt.Println("Extraction complete.")
	return nil
}

// createDesktopFile creates a .desktop file if it doesn't exist.
func createDesktopFile(homeDir, execPath string) error {
	// Define the target locations.
	desktopFileName := "discord.desktop"
	// Common locations for desktop/menu icons:
	desktopPath := filepath.Join(homeDir, "Desktop", desktopFileName)
	applicationsPath := filepath.Join(homeDir, ".local", "share", "applications", desktopFileName)

	// Template for the .desktop file.
	desktopFileContent := fmt.Sprintf(`[Desktop Entry]
Version=1.0
Type=Application
Name=Discord
Comment=Discord Chat Client
Exec=%s
Icon=%s
Terminal=false
Categories=Network;Chat;
`, execPath, execPath)

	// Function to create a file if it doesn't exist.
	createIfNotExists := func(path string) error {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Println("Creating desktop icon at", path)
			return os.WriteFile(path, []byte(desktopFileContent), 0755)
		} else {
			fmt.Println("Desktop icon already exists at", path)
		}
		return nil
	}

	// Create desktop and applications icons if needed.
	if err := createIfNotExists(desktopPath); err != nil {
		return err
	}
	if err := createIfNotExists(applicationsPath); err != nil {
		return err
	}

	return nil
}

func main() {
	// Determine the user's home directory.
	usr, err := user.Current()
	if err != nil {
		fmt.Println("Error getting current user:", err)
		return
	}
	homeDir := usr.HomeDir

	// Path settings.
	downloadURL := "https://discord.com/api/download?platform=linux&format=tar.gz"
	downloadPath := "/var/tmp/discord.tar.gz"
	discordDir := filepath.Join(homeDir, "Discord")

	fmt.Println("=== Discord Installer ===")

	// Step 0: Remove existing Discord folder.
	if err := removeDirIfExists(discordDir); err != nil {
		fmt.Println("Error removing old Discord folder:", err)
		return
	}

	// Step 1: Download.
	if err := downloadFile(downloadURL, downloadPath); err != nil {
		fmt.Println("Error during download:", err)
		return
	}

	// Step 2: Extract.
	// Extract to home directory first.
	if err := extractTarGz(downloadPath, homeDir); err != nil {
		fmt.Println("Error during extraction:", err)
		return
	}

	// Determine the extracted folder name.
	// This example assumes the tar archive has a top-level folder.
	// If not, you might need to adjust this logic.
	entries, err := os.ReadDir(homeDir)
	if err != nil {
		fmt.Println("Error reading home directory:", err)
		return
	}
	var extractedDir string
	for _, entry := range entries {
		// Look for a directory whose name contains "Discord" (case insensitive).
		if entry.IsDir() && strings.Contains(strings.ToLower(entry.Name()), "discord") {
			extractedDir = filepath.Join(homeDir, entry.Name())
			break
		}
	}
	if extractedDir == "" {
		fmt.Println("Error: Could not find extracted Discord folder.")
		return
	}

	// Rename the extracted directory to "Discord" if needed.
	if extractedDir != discordDir {
		fmt.Println("Renaming", extractedDir, "to", discordDir)
		if err := os.Rename(extractedDir, discordDir); err != nil {
			fmt.Println("Error renaming folder:", err)
			return
		}
	}

	// Step 3: Create desktop and menu icons.
	// The Discord binary is assumed to be at "<discordDir>/Discord".
	discordBinary := filepath.Join(discordDir, "Discord")
	if err := createDesktopFile(homeDir, discordBinary); err != nil {
		fmt.Println("Error creating desktop file:", err)
		return
	}

	fmt.Println("Installation complete!")
}
