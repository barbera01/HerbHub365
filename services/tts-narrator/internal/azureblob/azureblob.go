// Package azureblob uploads a local file to an Azure Blob Storage container
// via a SAS URL, using a plain HTTP PUT (no Azure SDK dependency).
package azureblob

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Upload PUTs the file at localPath to blobName within the container
// identified by blobSASURL (e.g. "https://acct.blob.core.windows.net/container?sv=...&sig=...").
func Upload(localPath, blobName, contentType, blobSASURL string) error {
	base, query, _ := strings.Cut(blobSASURL, "?")

	blobURL := strings.TrimRight(base, "/") + "/" + blobName
	if query != "" {
		blobURL += "?" + query
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, blobURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("blob upload failed: status %d", resp.StatusCode)
	}

	return nil
}

// PublicURL joins a container's public base URL with a blob name.
func PublicURL(blobPublicBase, blobName string) string {
	return strings.TrimRight(blobPublicBase, "/") + "/" + blobName
}
