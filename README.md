# Mini Orchestrator

**A Lightweight Kubernetes-Inspired Container Orchestration Platform Built from Scratch**

Mini Orchestrator is a lightweight container orchestration platform that demonstrates the core concepts behind Kubernetes, including scheduling, distributed state management, replica reconciliation, health monitoring, and container lifecycle management.

Built entirely from scratch using **Go**, **Docker**, **etcd**, and **React**, this project serves as an educational implementation of modern cloud-native orchestration systems.

---

## Table of Contents

- Overview
- Features
- Architecture
- Tech Stack
- Project Structure
- Getting Started
- API Endpoints
- Workflow
- Learning Outcomes
- Future Improvements
- Screenshots
- Contributors
- License

---

# Overview

Container orchestration platforms like Kubernetes automate the deployment, scheduling, monitoring, and scaling of containers across a cluster.

This project recreates those core concepts on a smaller scale to demonstrate how orchestration systems work internally.

Instead of relying on Kubernetes, Mini Orchestrator implements the orchestration logic itself, making it an excellent project for understanding distributed systems and cloud infrastructure.

---

#  Features

## API Server

- RESTful API
- Pod CRUD Operations
- Node Registration
- Replica Management
- Cluster State APIs
- Event APIs

---

## Scheduler

Supports intelligent pod placement using:

- Round Robin Scheduling
- CPU-Aware Scheduling
- Dynamic Node Discovery
- Scheduling Decision Logging

---

## Node Agent

Runs on every worker node.

Responsibilities include:

- Container Creation
- Docker Integration
- Health Reporting
- Pod Status Updates
- Heartbeat Monitoring
- Log Collection

---

## Replica Controller

Maintains desired cluster state by:

- Replica Reconciliation
- Failed Pod Recreation
- Scaling Support
- Desired vs Actual State Comparison

---

## Distributed State Store

Powered by **etcd**

Stores:

- Pods
- Nodes
- ReplicaSets
- Scheduling Metadata
- Cluster State
- Pod Status

---

## Monitoring Dashboard

Features include:

- Cluster Overview
- Pod Monitoring
- Node Monitoring
- Live Logs
- Resource Statistics
- Replica Visualization

---

# System Architecture

```text
                    +----------------------+
                    |   React Dashboard    |
                    +----------+-----------+
                               |
                         REST API Calls
                               |
                    +----------v-----------+
                    |      API Server      |
                    +----------+-----------+
                               |
        ------------------------------------------------
        |                    |                        |
        |                    |                        |
    Scheduler        Replica Controller        Event Manager
                \            |              /
                 \           |             /
                  +----------v------------+
                  |        etcd           |
                  +----------+------------+
                             |
      -------------------------------------------------
      |                    |                         |
      |                    |                         |
  Node Agent 1        Node Agent 2            Node Agent N
      |                    |                         |
 Docker Engine        Docker Engine           Docker Engine
```

---

# Technology Stack

| Category | Technology |
|----------|------------|
| Language | Go |
| Frontend | React + TypeScript |
| Styling | Tailwind CSS |
| API | REST |
| Container Runtime | Docker |
| Distributed Store | etcd |
| Build Tool | Vite |

---

# Project Structure

```text
mini-orchestrator/

├── backend/
│   ├── api/
│   ├── scheduler/
│   ├── controller/
│   ├── nodeagent/
│   ├── models/
│   ├── storage/
│   ├── utils/
│   ├── main.go
│   └── go.mod
│
├── frontend/
│   ├── src/
│   ├── public/
│   └── package.json
│
├── docker-compose.yml
├── README.md
└── LICENSE
```

---

# Getting Started

## Clone Repository

```bash
git clone https://github.com/YOUR_USERNAME/mini-orchestrator.git

cd mini-orchestrator
```

---

## Start etcd

```bash
docker run -d \
--name etcd \
-p 2379:2379 \
-p 2380:2380 \
quay.io/coreos/etcd:v3.5.12 \
/usr/local/bin/etcd \
--advertise-client-urls=http://0.0.0.0:2379 \
--listen-client-urls=http://0.0.0.0:2379
```

---

## Run Backend

```bash
cd backend

go mod tidy

go run .
```

---

## Run Frontend

```bash
cd frontend

npm install

npm run dev
```

---

# API Endpoints

## Pods

| Method | Endpoint | Description |
|---------|----------|-------------|
| GET | `/pods` | Retrieve all Pods |
| POST | `/pods` | Create a Pod |
| DELETE | `/pods/{name}` | Delete a Pod |

---

## Nodes

| Method | Endpoint | Description |
|---------|----------|-------------|
| GET | `/nodes` | List Nodes |
| POST | `/nodes` | Register Node |

---

## ReplicaSets

| Method | Endpoint | Description |
|---------|----------|-------------|
| GET | `/replicas` | Retrieve ReplicaSets |
| POST | `/replicas` | Create ReplicaSet |

---

## Logs

| Method | Endpoint | Description |
|---------|----------|-------------|
| GET | `/logs/{pod}` | Fetch Container Logs |

---

## Events

| Method | Endpoint | Description |
|---------|----------|-------------|
| GET | `/events` | Cluster Events |

---

# Workflow

```text
Register Node
      │
      ▼
Submit Pod
      │
      ▼
Scheduler Chooses Best Node
      │
      ▼
Store Pod Metadata in etcd
      │
      ▼
Node Agent Watches etcd
      │
      ▼
Docker Creates Container
      │
      ▼
Container Status Updated
      │
      ▼
Dashboard Reflects Cluster State
```

---

## Learning Outcomes

This project demonstrates practical knowledge of:

- Distributed Systems
- Container Orchestration
- Docker Internals
- Scheduling Algorithms
- Distributed Key-Value Stores
- REST API Development
- Backend System Design
- Cluster State Reconciliation
- Health Monitoring
- Cloud-Native Architecture

---

# Future Improvements

- Horizontal Pod Autoscaling
- Rolling Updates
- Canary Deployments
- Service Discovery
- Persistent Volumes
- Leader Election
- Multi-Master Support
- Resource Quotas
- Metrics Server
- Prometheus Integration
- Grafana Dashboard
- Authentication & RBAC
- YAML Deployment Support
- Helm-like Package Manager
- WebSocket Live Updates

---

#  Screenshots

> Add screenshots of your dashboard here.

Example:

```
docs/

├── dashboard.png
├── pods.png
├── nodes.png
├── logs.png
└── metrics.png
```

---

# Contributors

### **Kartikay Srivastava**

**Computer Science Engineering Student**

**Interests**

- Distributed Systems
- Cloud Computing
- Backend Development
- System Design

---

# Inspiration

Mini Orchestrator is inspired by Kubernetes and aims to recreate its fundamental orchestration concepts in a lightweight educational implementation. The project focuses on understanding the internal workings of modern cloud-native infrastructure rather than replacing production-grade orchestration platforms.

---

#  License

This project is licensed under the **MIT License**.

---

## Support

If you found this project helpful, consider giving it a **Star** on GitHub. It helps others discover the project and motivates further development.
