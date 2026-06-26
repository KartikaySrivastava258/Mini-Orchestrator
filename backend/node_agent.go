package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)
import (
	clientv3 "go.etcd.io/etcd/client/v3"
	"context"
)

type Node struct {
	Name          string `json:"name"`
	Active        bool   `json:"active"`
	LastHeartbeat int64  `json:"last_heartbeat"`
    CPUUsage      float64 `json:"cpu_usage"`
}

type Pod struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Node   string `json:"node"`
	Status string `json:"status"`
}

var serverURL = "http:
var client *clientv3.Client

var runningPods = make(map[string]bool)
var restartCount = make(map[string]int)

func registerNode(nodeName string) {
	node := Node{
		Name:          nodeName,
		Active:        true,
		LastHeartbeat: time.Now().Unix(),
        CPUUsage:      0.0,
	}

	data, _ := json.Marshal(node)

	resp, err := http.Post(serverURL+"/nodes", "application/json", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println("Error registering node:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Node registered:", nodeName)
}

func sendHeartbeat(nodeName string) {
	for {

		cpu := getCPUUsage()

		node := Node{
			Name:     nodeName,
			CPUUsage: cpu,
		}

		data, _ := json.Marshal(node)

		resp, err := http.Post(serverURL+"/heartbeat", "application/json", bytes.NewBuffer(data))
		if err != nil {
			fmt.Println(" Heartbeat failed:", err)
		} else {
			fmt.Println("Heartbeat sent | CPU:", cpu, "%")
			resp.Body.Close()
		}

		time.Sleep(3 * time.Second)
	}
}

func getPods() ([]Pod, error) {
	resp, err := http.Get(serverURL + "/getpods")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pods []Pod
	err = json.NewDecoder(resp.Body).Decode(&pods)
	return pods, err
}

func isContainerRunning(podName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "--filter", "name="+podName, "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return count(output) > 0
}

func isContainerActive(podName string) bool {
	cmd := exec.Command("docker", "ps", "--filter", "name="+podName, "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return count(output) > 0
}

func getContainerExitCode(podName string) (int, error) {
	cmd := exec.Command("docker", "inspect", podName, "--format", "{{.State.ExitCode}}")
	output, err := cmd.Output()
	if err != nil {
		return -1, err
	}

	var exitCode int
	fmt.Sscanf(string(output), "%d", &exitCode)

	return exitCode, nil
}

func getCPUUsage() float64 {

	cmd := exec.Command("bash", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2 + $4}'")
	output, err := cmd.Output()
	if err != nil {
		return 0.0
	}

	var usage float64
	fmt.Sscanf(string(output), "%f", &usage)

	return usage
}

func runContainer(pod Pod) error {

	fmt.Println("Starting container:", pod.Name)

	cmd := exec.Command("docker", "run", "-d", "--name", pod.Name, pod.Image)
	err := cmd.Run()

	if err != nil {
		fmt.Println("Docker run failed:", err)
		return err
	}

	fmt.Println("Container started:", pod.Name)
	return nil
}

func stopContainer(podName string) {
	cmd := exec.Command("docker", "rm", "-f", podName)
	err := cmd.Run()

	if err != nil {
		fmt.Println("Failed to stop container:", podName)
		return
	}

	fmt.Println("Container stopped:", podName)
}

func updatePodStatus(podName string, status string) {

	payload := map[string]string{
		"name":   podName,
		"status": status,
	}

	data, _ := json.Marshal(payload)

	resp, err := http.Post(serverURL+"/updatePodStatus", "application/json", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println(" Failed to update status:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Pod status updated:", podName, "→", status)
}

func watchPods(nodeName string) {

	for {
		watchChan := client.Watch(context.Background(), "/pods/", clientv3.WithPrefix())

		fmt.Println(" Node Agent watching for pod changes...")

		for watchResp := range watchChan {

			if watchResp.Err() != nil {
				fmt.Println("Watch error:", watchResp.Err())
				continue
			}

			for _, event := range watchResp.Events {

				if event.Type == clientv3.EventTypeDelete {

					podName := string(event.Kv.Key[count("/pods/"):])

					if isContainerRunning(podName) {
						fmt.Println("Cleaning container:", podName)
						stopContainer(podName)
					}
					delete(runningPods, podName)
					continue
				}

				if event.Type == clientv3.EventTypePut {

					var pod Pod

					err := json.Unmarshal(event.Kv.Value, &pod)
					if err != nil {
						fmt.Println(" Failed to parse pod:", err)
						continue
					}

					fmt.Println("Event received for pod:", pod.Name)

					if pod.Node != nodeName || pod.Status != "Running" {
						continue
					}

					if isContainerRunning(pod.Name) && isContainerActive(pod.Name) {
						runningPods[pod.Name] = true
						continue
					}

                    if isContainerRunning(pod.Name) && !isContainerActive(pod.Name) {

                    exitCode, err := getContainerExitCode(pod.Name)
	                if err != nil {
		            fmt.Println("Failed to get exit code:", err)
		            continue
	                }

	                if exitCode == 0 {
		            fmt.Println("Container completed successfully:", pod.Name)
		            updatePodStatus(pod.Name, "Succeeded")
		            delete(runningPods, pod.Name)
                    delete(restartCount, pod.Name)
		            continue
	                }

	                restartCount[pod.Name]++

	                fmt.Println(" Container failed:", pod.Name, "| Restart count:", restartCount[pod.Name])

	                if restartCount[pod.Name] >= 3 {
		            fmt.Println(" CrashLoopBackOff for pod:", pod.Name)
		            updatePodStatus(pod.Name, "CrashLoopBackOff")
                    delete(runningPods, pod.Name)
		            continue
	                }

	                fmt.Println(" Restarting container:", pod.Name)
	                stopContainer(pod.Name)
                    }

					err = runContainer(pod)
					if err != nil {
						updatePodStatus(pod.Name, "Failed")
						continue
					}
                   runningPods[pod.Name] = true
                   restartCount[pod.Name] = 0
                   updatePodStatus(pod.Name, "Running")
				}
			}
		}

		fmt.Println("Watch channel closed, restarting...")
		time.Sleep(2 * time.Second)
	}
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

	fmt.Println("Connected to etcd")
}

func main() {

	if count(os.Args) < 2 {
		fmt.Println("Usage: go run node-agent.go <node-name>")
		return
	}

	nodeName := os.Args[1]

	fmt.Println(" Node Agent started for:", nodeName)
    initEtcd()

	registerNode(nodeName)

	go sendHeartbeat(nodeName)

	watchPods(nodeName)
}
