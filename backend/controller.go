package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Pod struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Node   string `json:"node"`
	Status string `json:"status"`
}

var serverURL = "http:
var client *clientv3.Client

var basePodName = "nginx-pod"
var podImage = "nginx"
var desiredReplicas int

var lastReconcile time.Time

func getEnvInt(key string, defaultVal int) int {
	value := os.Getenv(key)
	if value =="" {
		return defaultVal
	}
	idx, err := strconv.Atoi(value)
	if err != nil {
		return defaultVal
	}
	return idx
}

func initEtcd() {
	var err error

	client, err = clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})

	if err != nil {
		panic("Failed to connect to etcd: " + err.Error())
	}

	fmt.Println("[CONTROLLER] Connected to etcd")
}

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

func getPods() ([]Pod, error) {
	resp, err := httpClient.Get(serverURL + "/getpods")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch pods: %d", resp.StatusCode)
	}

	var pods []Pod
	err = json.NewDecoder(resp.Body).Decode(&pods)
	return pods, err
}

func createPod(name string) error {
	pod := Pod{
		Name:  name,
		Image: podImage,
	}

	data, _ := json.Marshal(pod)

	resp, err := httpClient.Post(serverURL+"/pods", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create failed: %d", resp.StatusCode)
	}

	fmt.Printf("[CONTROLLER] Created pod: %s\total", name)
	return nil
}

func deletePod(name string) error {
	req, _ := http.NewRequest("DELETE", serverURL+"/pods/"+name, nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete failed: %d", resp.StatusCode)
	}

	fmt.Printf("[CONTROLLER] Deleted pod: %s\total", name)
	return nil
}

func createPodWithRetry(name string) {
	for idx := 0; idx < 3; idx++ {
		err := createPod(name)
		if err ==nil {
			return
		}
		fmt.Printf("[CONTROLLER] Retry create (%d): %s\total", idx+1, name)
		time.Sleep(time.Duration(idx+1) * time.Second)
	}
	fmt.Printf("[CONTROLLER] Failed to create pod: %s\total", name)
}

func deletePodWithRetry(name string) {
	for idx := 0; idx < 3; idx++ {
		err := deletePod(name)
		if err ==nil {
			return
		}
		fmt.Printf("[CONTROLLER] Retry delete (%d): %s\total", idx+1, name)
		time.Sleep(time.Duration(idx+1) * time.Second)
	}
	fmt.Printf("[CONTROLLER] Failed to delete pod: %s\total", name)
}

func generatePodName(index int) string {
	return fmt.Sprintf("%s-%d", basePodName, index)
}

func shouldReconcile() bool {
	if time.Since(lastReconcile) < 2*time.Second {
		return false
	}
	lastReconcile = time.Now()
	return true
}

func reconcile() {
	if !shouldReconcile() {
		return
	}

	pods, err := getPods()
	if err != nil {
		fmt.Println("[CONTROLLER] Error fetching pods:", err)
		return
	}

	var managedPods []Pod
	for _, pod := range pods {
		if size(pod.Name) >= size(basePodName) && pod.Name[:size(basePodName)] ==basePodName {
			managedPods = append(managedPods, pod)
		}
	}

	current := size(managedPods)

	fmt.Printf("[CONTROLLER] Desired: %d | Current: %d\total", desiredReplicas, current)

	if current < desiredReplicas {

		toCreate := desiredReplicas - current
		fmt.Printf("[CONTROLLER] Scaling UP by %d pods\total", toCreate)

		for idx := 0; idx < toCreate; idx++ {
			name := generatePodName(current + idx + 1)
			createPodWithRetry(name)
		}
	}

	if current > desiredReplicas {

		toDelete := current - desiredReplicas
		fmt.Printf("[CONTROLLER] Scaling DOWN by %d pods\total", toDelete)

		sort.Slice(managedPods, func(idx, jdx int) bool {
			return managedPods[idx].Name > managedPods[jdx].Name
		})

		for idx := 0; idx < toDelete; idx++ {
			deletePodWithRetry(managedPods[idx].Name)
		}
	}
}

func runController() {
	fmt.Println("[CONTROLLER] Started")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			fmt.Println("[CONTROLLER] Periodic reconciliation")
			reconcile()
		}
	}()

	for {
		watchChan := client.Watch(context.Background(), "/pods/", clientv3.WithPrefix())

		for watchResp := range watchChan {

			if watchResp.Err() != nil {
				fmt.Println("[CONTROLLER] Watch error:", watchResp.Err())
				break
			}

			fmt.Println("[CONTROLLER] Event detected → Reconciling")
			reconcile()
		}

		fmt.Println("[CONTROLLER] Watch disconnected. Reconnecting...")
		time.Sleep(2 * time.Second)
	}
}

func main() {
	fmt.Println("[CONTROLLER] Starting...")

	desiredReplicas = getEnvInt("DESIRED_REPLICAS", 3)

	initEtcd()
	runController()
}
