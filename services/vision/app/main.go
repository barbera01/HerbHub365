package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	rabbitmqURL = "https://rabbit.herbhub365.com" // RabbitMQ API URL
	username    = "admin"                         // RabbitMQ username
	password    = "yourpassword"                  // RabbitMQ password
	queueName   = "TimeLapse"                     // Queue name
	vhost       = "/"                             // Virtual host
)

type QueueInfo struct {
	Messages int `json:"messages"`
}

// TimeLapseRequest represents the expected payload format
type TimeLapseRequest struct {
	Command string `json:"command"`
}

func main() {
	// Get message count from the queue
	messageCount, err := getMessageCount()
	if err != nil {
		log.Fatalf("Error fetching queue info: %v", err)
	}
	fmt.Printf("Message Count: %d\n", messageCount)

	if messageCount > 0 {
		// Get the newest message
		payload, err := getNewestMessage()
		if err != nil {
			log.Fatalf("Error fetching message: %v", err)
		}

		fmt.Println("Newest Message Payload:")
		fmt.Println(payload)

		// Attempt to parse the payload as TimeLapseRequest
		var timeLapseReq TimeLapseRequest
		err = json.Unmarshal([]byte(payload), &timeLapseReq)
		if err != nil {
			log.Printf("Payload is not a valid TimeLapseRequest: %v", err)
			return
		}

		// Run the rpicam-still command
		err = runRpiCamStill()
		if err != nil {
			log.Fatalf("Error running rpicam-still: %v", err)
		}
		fmt.Println("rpicam-still command executed successfully.")
	} else {
		fmt.Println("No messages in the queue.")
	}
}

func getMessageCount() (int, error) {
	url := fmt.Sprintf("%s/api/queues/%s/%s", rabbitmqURL, "%2F", queueName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("error creating request: %v", err)
	}
	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var queueInfo QueueInfo
	err = json.NewDecoder(resp.Body).Decode(&queueInfo)
	if err != nil {
		return 0, fmt.Errorf("error decoding response: %v", err)
	}
	return queueInfo.Messages, nil
}

func getNewestMessage() (string, error) {
	url := fmt.Sprintf("%s/api/queues/%s/%s/get", rabbitmqURL, "%2F", queueName)
	body := map[string]interface{}{
		"count":    1,
		"ackmode":  "ack_requeue_false",
		"encoding": "auto",
		"truncate": 50000,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(responseBody))
	}

	var messages []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&messages)
	if err != nil {
		return "", fmt.Errorf("error decoding response: %v", err)
	}

	if len(messages) > 0 {
		payload, ok := messages[0]["payload"].(string)
		if !ok {
			return "", fmt.Errorf("unexpected payload format")
		}
		return payload, nil
	}

	return "", fmt.Errorf("no messages returned")
}

func runRpiCamStill() error {
	// Directory where the image will be saved
	outputDir := "/images/"

	// Generate the filename
	filename, err := generateFilename(outputDir)
	if err != nil {
		return fmt.Errorf("error generating filename: %w", err)
	}

	// Build the rpicam-still command
	cmd := exec.Command("rpicam-still",
		"-o", outputDir+filename,
		"--post-process-file", "/usr/share/rpi-camera-assets/imx500_mobilenet_ssd.json",
		"--width", "1920",
		"--height", "1080",
		"--shutter", "20000",
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	return cmd.Run()
}

func generateFilename(dir string) (string, error) {
	// Get the current date
	currentDate := time.Now().Format("02-01-2006") // dd-mm-yyyy format

	// Count the number of .jpg files in the directory
	fileCount, err := countFilesWithExtension(dir, ".jpg")
	if err != nil {
		return "", fmt.Errorf("error counting files: %w", err)
	}

	// Generate the filename
	return fmt.Sprintf("%d-%s.jpg", fileCount+1, currentDate), nil
}

func countFilesWithExtension(dir, extension string) (int, error) {
	count := 0

	// Walk through the directory to count files with the given extension
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(info.Name()) == extension {
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return count, nil
}
