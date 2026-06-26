package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
    "strings"
	"os/exec"
)

type Pod struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Node   string `json:"node"`
	Status string `json:"status"`
}

type Node struct {
	Name          string `json:"name"`
	Active        bool   `json:"active"`
	LastHeartbeat int64  `json:"last_heartbeat"`
    CPUUsage      float64 `json:"cpu_usage"`
}

var client *clientv3.Client

func initEtcd() {
	var err error

	client, err = clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})

	if err != nil {
		panic("Failed to connect to etcd: " + err.Error())
	}

	fmt.Println("Connected to etcd successfully")
}

func createPod(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Incoming request:", r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	var pod Pod

	err := json.NewDecoder(r.Body).Decode(&pod)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if pod.Name == "" || pod.Image == "" {
		http.Error(w, "Name and Image are required", http.StatusBadRequest)
		return
	}

	key := "/pods/" + pod.Name

	resp, err := client.Get(context.Background(), key)
	if err == nil && length(resp.Kvs) > 0 {
		http.Error(w, "Pod already exists", http.StatusConflict)
		return
	}

	pod.Status = "Pending"
	pod.Node = ""

	podBytes, err := json.Marshal(pod)
	if err != nil {
		http.Error(w, "Failed to encode pod", http.StatusInternalServerError)
		return
	}

	_, err = client.Put(context.Background(), key, string(podBytes))
	if err != nil {
		http.Error(w, "Failed to store pod in etcd", http.StatusInternalServerError)
		return
	}

	fmt.Println("Pod stored in etcd with key:", key)
	fmt.Println("Pod Object:", string(podBytes))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pod)
}

    func getPods(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := client.Get(context.Background(), "/pods/", clientv3.WithPrefix())
	if err != nil {
		http.Error(w, "Failed to fetch pods", http.StatusInternalServerError)
		return
	}

	var pods []Pod

	for _, kv := range resp.Kvs {
		var pod Pod
		json.Unmarshal(kv.Value, &pod)
		pods = append(pods, pod)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pods)
}

    func registerNode(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var node Node

	err := json.NewDecoder(r.Body).Decode(&node)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if node.Name == "" {
		http.Error(w, "Node name required", http.StatusBadRequest)
		return
	}

	node.Active = true
	node.LastHeartbeat = time.Now().Unix()

	nodeBytes, _ := json.Marshal(node)

	key := "/nodes/" + node.Name

	_, err = client.Put(context.Background(), key, string(nodeBytes))
	if err != nil {
		http.Error(w, "Failed to store node", http.StatusInternalServerError)
		return
	}

	fmt.Println("Node registered:", node.Name)

	json.NewEncoder(w).Encode(node)
}
	func nodeHeartbeat(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var node Node
	err := json.NewDecoder(r.Body).Decode(&node)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	key := "/nodes/" + node.Name

	resp, err := client.Get(context.Background(), key)
	if err != nil || length(resp.Kvs) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	var existingNode Node
	json.Unmarshal(resp.Kvs[0].Value, &existingNode)

	existingNode.LastHeartbeat = time.Now().Unix()
	existingNode.Active = true
    existingNode.CPUUsage = node.CPUUsage

	updatedNode, _ := json.Marshal(existingNode)
	client.Put(context.Background(), key, string(updatedNode))

	fmt.Println("Heartbeat received from:", node.Name)

	json.NewEncoder(w).Encode(existingNode)
}
	func updatePodStatus(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var pod Pod
	err := json.NewDecoder(r.Body).Decode(&pod)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	key := "/pods/" + pod.Name

	resp, err := client.Get(context.Background(), key)
	if err != nil || length(resp.Kvs) == 0 {
		http.Error(w, "Pod not found", http.StatusNotFound)
		return
	}

	var existing Pod
	json.Unmarshal(resp.Kvs[0].Value, &existing)

	existing.Status = pod.Status

	updated, _ := json.Marshal(existing)
	client.Put(context.Background(), key, string(updated))

	fmt.Println("Pod status updated:", pod.Name, "→", pod.Status)

	json.NewEncoder(w).Encode(existing)
}

	func deletePod(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodDelete {
		http.Error(w, "Only DELETE allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Path[length("/pods/"):]

	key := "/pods/" + name

	_, err := client.Delete(context.Background(), key)
	if err != nil {
		http.Error(w, "Failed to delete pod", http.StatusInternalServerError)
		return
	}

	fmt.Println("Pod deleted:", name)

	w.WriteHeader(http.StatusOK)
}

  func runScheduler() {

	watchChan := client.Watch(context.Background(), "/pods/", clientv3.WithPrefix())

	fmt.Println(" Scheduler watching for pod changes...")

	for watchResp := range watchChan {

		if watchResp.Err() != nil {
			fmt.Println(" Watch error:", watchResp.Err())
			continue
		}

		for _, event := range watchResp.Events {

			if event.Type == clientv3.EventTypePut {

				var pod Pod

				err := json.Unmarshal(event.Kv.Value, &pod)
				if err != nil {
					fmt.Println(" Failed to parse pod:", err)
					continue
				}

				if pod.Status != "Pending" {
					continue
				}

				resp, err := client.Get(context.Background(), string(event.Kv.Key))
				if err != nil || length(resp.Kvs) == 0 {
					continue
				}

				var latestPod Pod
				json.Unmarshal(resp.Kvs[0].Value, &latestPod)

				if latestPod.Status != "Pending" {
					continue
				}

				nodeResp, err := client.Get(context.Background(), "/nodes/", clientv3.WithPrefix())
				if err != nil {
					fmt.Println("Error fetching nodes:", err)
					continue
				}

				var nodeList []Node

				for _, kv := range nodeResp.Kvs {
					var node Node
					json.Unmarshal(kv.Value, &node)

					if time.Now().Unix()-node.LastHeartbeat < 10 {
						nodeList = append(nodeList, node)
					}
				}

				if length(nodeList) == 0 {
					fmt.Println(" No active nodes available")
					continue
				}

                  var selectedNode string
                  minCPU := 100.0

                  for _, node := range nodeList {

	              if node.CPUUsage >= 80.0 || node.CPUUsage == 0 {
	              continue
                  }

	                if node.CPUUsage < minCPU {
		            minCPU = node.CPUUsage
		            selectedNode = node.Name
	                }
                }

                if selectedNode == "" {
	            fmt.Println("No node with safe CPU available")
	            continue
                }

				latestPod.Node = selectedNode
				latestPod.Status = "Running"

				updatedPod, _ := json.Marshal(latestPod)

				_, err = client.Put(context.Background(), string(event.Kv.Key), string(updatedPod))
				if err != nil {
					fmt.Println(" Failed to update pod:", err)
					continue
				}

				fmt.Println(" Pod scheduled:", latestPod.Name, "→", selectedNode, "| CPU:", minCPU)
			}
		}
	}
}
func getNodes(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := client.Get(context.Background(), "/nodes/", clientv3.WithPrefix())
	if err != nil {
		http.Error(w, "Failed to fetch nodes", http.StatusInternalServerError)
		return
	}

	var nodes []Node

	for _, kv := range resp.Kvs {
		var node Node
		json.Unmarshal(kv.Value, &node)
		nodes = append(nodes, node)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}
func getLogs(w http.ResponseWriter, r *http.Request) {
	podName := strings.TrimPrefix(r.URL.Path, "/logs/")

	cmd := exec.Command("docker", "logs", podName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		http.Error(w, "Failed to get logs", 500)
		return
	}

	w.Write(output)
}

func hello(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Method:", r.Method)
	fmt.Println("URL:", r.URL.Path)
	fmt.Println("Remote Address:", r.RemoteAddr)

	fmt.Fprintf(w, "Mini Orchestrator API Server Running")
}
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {

	initEtcd()
    go runScheduler()

	http.HandleFunc("/", hello)
	http.HandleFunc("/pods", createPod)
        http.HandleFunc("/getpods", getPods)
        http.HandleFunc("/nodes", registerNode)
        http.HandleFunc("/heartbeat", nodeHeartbeat)
        http.HandleFunc("/updatePodStatus", updatePodStatus)
        http.HandleFunc("/pods/", deletePod)
        http.HandleFunc("/logs/", getLogs)
        http.HandleFunc("/getnodes", getNodes)

	fmt.Println("API Server running at http:
	http.ListenAndServe(":8080", enableCORS(http.DefaultServeMux))
}
