package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func main() {
	const (
		namespace   = "fuyuu-router"
		label       = "app=curl-tester"
		concurrency = 20
		routerURL   = "http://fuyuu-router-hub.fuyuu-router:8080"
		contentType = "Content-Type: application/json"
	)

	getPodNameCmd := fmt.Sprintf("kubectl get pods -n %s -l %s -o jsonpath='{.items[0].metadata.name}'", namespace, label)
	curlTesterPod, err := exec.Command("bash", "-c", getPodNameCmd).Output()
	if err != nil {
		fmt.Printf("Failed to get pod name: %s\n", err)
		os.Exit(1)
	}
	podName := strings.TrimSpace(string(curlTesterPod))

	var wg sync.WaitGroup
	wg.Add(concurrency)

	errors := make(chan string, concurrency)

	for i := 1; i <= concurrency; i++ {
		go func(i int) {
			defer wg.Done()

			jsonData := map[string]interface{}{
				"message": fmt.Sprintf("This is request %d", i),
			}
			jsonBytes, err := json.Marshal(jsonData)
			if err != nil {
				errors <- fmt.Sprintf("Failed to marshal JSON: %s", err)
				return
			}

			response, err := exec.Command("kubectl", "exec", "-n", namespace, podName, "--",
				"curl", "-X", "POST", "-H", "FuyuuRouter-IDs: agent01", "-H", contentType, "-d", string(jsonBytes), routerURL, "-s").Output()

			var responseJSON map[string]interface{}
			if err := json.Unmarshal(response, &responseJSON); err != nil {
				errors <- fmt.Sprintf("Failed to unmarshal JSON response: %s\nResponse: %s", err, response)
				return
			}

			expectedMessage, _ := jsonData["message"].(string)
			if responseMessage, ok := responseJSON["message"].(string); err != nil || !ok || responseMessage != expectedMessage {
				errorMsg := fmt.Sprintf("Request and response do not match:\nRequest: %s\nResponse: %s\n", jsonData, string(response))
				errors <- errorMsg
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for errorMsg := range errors {
		fmt.Println(errorMsg)
		errorCount++
	}

	if errorCount == 0 {
		fmt.Println("All requests returned the correct response.")
	} else {
		fmt.Printf("Some requests did not return the correct response. Errors: %d\n", errorCount)
		os.Exit(1)
	}
}
