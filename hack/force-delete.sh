#!/bin/bash

# Force Delete Script for Stuck Resources
# This script helps to force delete pods, PVCs, and PVs that are stuck in terminating state

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Shared Volume Controller - Force Delete Utility${NC}"
echo "=============================================="

# Function to force delete a pod
force_delete_pod() {
    local pod_name=$1
    local namespace=$2
    
    echo -e "${YELLOW}Force deleting pod: ${pod_name} in namespace: ${namespace}${NC}"
    
    # Remove finalizers from pod
    kubectl patch pod "${pod_name}" -n "${namespace}" -p '{"metadata":{"finalizers":[]}}' --type=merge
    
    # Force delete with grace period 0
    kubectl delete pod "${pod_name}" -n "${namespace}" --force --grace-period=0
    
    echo -e "${GREEN}Pod force deletion initiated${NC}"
}

# Function to force delete a PVC
force_delete_pvc() {
    local pvc_name=$1
    local namespace=$2
    
    echo -e "${YELLOW}Force deleting PVC: ${pvc_name} in namespace: ${namespace}${NC}"
    
    # Remove finalizers from PVC
    kubectl patch pvc "${pvc_name}" -n "${namespace}" -p '{"metadata":{"finalizers":[]}}' --type=merge
    
    # Force delete
    kubectl delete pvc "${pvc_name}" -n "${namespace}" --force --grace-period=0
    
    echo -e "${GREEN}PVC force deletion initiated${NC}"
}

# Function to force delete a PV
force_delete_pv() {
    local pv_name=$1
    
    echo -e "${YELLOW}Force deleting PV: ${pv_name}${NC}"
    
    # Remove finalizers from PV
    kubectl patch pv "${pv_name}" -p '{"metadata":{"finalizers":[]}}' --type=merge
    
    # Force delete
    kubectl delete pv "${pv_name}" --force --grace-period=0
    
    echo -e "${GREEN}PV force deletion initiated${NC}"
}

# Function to show stuck resources
show_stuck_resources() {
    echo -e "${YELLOW}Checking for stuck resources...${NC}"
    echo ""
    
    echo "Pods stuck in Terminating state:"
    kubectl get pods --all-namespaces --field-selector=status.phase=Terminating
    echo ""
    
    echo "PVCs with finalizers and deletion timestamp:"
    kubectl get pvc --all-namespaces -o custom-columns="NAMESPACE:.metadata.namespace,NAME:.metadata.name,STATUS:.status.phase,FINALIZERS:.metadata.finalizers,DELETION:.metadata.deletionTimestamp" | grep -E "(DELETION|[0-9]{4}-[0-9]{2}-[0-9]{2})"
    echo ""
    
    echo "PVs with finalizers and deletion timestamp:"
    kubectl get pv -o custom-columns="NAME:.metadata.name,STATUS:.status.phase,FINALIZERS:.metadata.finalizers,DELETION:.metadata.deletionTimestamp" | grep -E "(DELETION|[0-9]{4}-[0-9]{2}-[0-9]{2})"
    echo ""
}

# Function to force delete resources by SharedVolume annotation
force_delete_by_sharedvolume() {
    local shared_volume_name=$1
    local namespace=$2
    
    echo -e "${YELLOW}Force deleting all resources for SharedVolume: ${shared_volume_name} in namespace: ${namespace}${NC}"
    
    # Find and delete pods with SharedVolume annotation
    echo "Searching for pods with SharedVolume annotations..."
    local pods=$(kubectl get pods --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{" "}{.metadata.annotations.sharedvolume\.sv/'"${shared_volume_name}"'}{"\n"}{end}' | grep "true" | awk '{print $1 ":" $2}')
    
    if [ -n "$pods" ]; then
        echo "Found pods with SharedVolume annotation:"
        for pod_info in $pods; do
            local pod_namespace=$(echo $pod_info | cut -d':' -f1)
            local pod_name=$(echo $pod_info | cut -d':' -f2)
            echo "  - $pod_name (namespace: $pod_namespace)"
            force_delete_pod "$pod_name" "$pod_namespace"
        done
    else
        echo "No pods found with SharedVolume annotation"
    fi
    
    # Find and delete PVCs that match naming patterns
    echo "Searching for related PVCs..."
    local pvc_patterns=("${shared_volume_name}" "${shared_volume_name}-${namespace}" "${shared_volume_name}-*")
    
    for pattern in "${pvc_patterns[@]}"; do
        local pvcs=$(kubectl get pvc --all-namespaces -o custom-columns="NAMESPACE:.metadata.namespace,NAME:.metadata.name" --no-headers | grep -E "${pattern}")
        if [ -n "$pvcs" ]; then
            while IFS= read -r line; do
                local pvc_namespace=$(echo "$line" | awk '{print $1}')
                local pvc_name=$(echo "$line" | awk '{print $2}')
                echo "Found related PVC: $pvc_name (namespace: $pvc_namespace)"
                force_delete_pvc "$pvc_name" "$pvc_namespace"
            done <<< "$pvcs"
        fi
    done
    
    # Find and delete PVs that match naming patterns
    echo "Searching for related PVs..."
    local pv_patterns=("pv-${shared_volume_name}" "${shared_volume_name}" "${shared_volume_name}-${namespace}" "${shared_volume_name}-*")
    
    for pattern in "${pv_patterns[@]}"; do
        local pvs=$(kubectl get pv -o custom-columns="NAME:.metadata.name" --no-headers | grep -E "${pattern}")
        if [ -n "$pvs" ]; then
            while IFS= read -r pv_name; do
                echo "Found related PV: $pv_name"
                force_delete_pv "$pv_name"
            done <<< "$pvs"
        fi
    done
    
    # Find and delete ReplicaSets that match
    echo "Searching for related ReplicaSets..."
    local replicasets=$(kubectl get rs -n "${namespace}" -o custom-columns="NAME:.metadata.name" --no-headers | grep -E "${shared_volume_name}")
    if [ -n "$replicasets" ]; then
        while IFS= read -r rs_name; do
            echo "Found related ReplicaSet: $rs_name (namespace: $namespace)"
            kubectl patch rs "${rs_name}" -n "${namespace}" -p '{"metadata":{"finalizers":[]}}' --type=merge
            kubectl delete rs "${rs_name}" -n "${namespace}" --force --grace-period=0
        done <<< "$replicasets"
    fi
    
    # Find and delete Services that match
    echo "Searching for related Services..."
    local services=$(kubectl get svc -n "${namespace}" -o custom-columns="NAME:.metadata.name" --no-headers | grep -E "${shared_volume_name}")
    if [ -n "$services" ]; then
        while IFS= read -r svc_name; do
            echo "Found related Service: $svc_name (namespace: $namespace)"
            kubectl delete svc "${svc_name}" -n "${namespace}" --force --grace-period=0
        done <<< "$services"
    fi
    
    # Find and delete NFS Servers that match
    echo "Searching for related NFS Servers..."
    local nfs_servers=$(kubectl get nfsserver -n "${namespace}" -o custom-columns="NAME:.metadata.name" --no-headers 2>/dev/null | grep -E "${shared_volume_name}" || true)
    if [ -n "$nfs_servers" ]; then
        while IFS= read -r nfs_name; do
            echo "Found related NFS Server: $nfs_name (namespace: $namespace)"
            kubectl patch nfsserver "${nfs_name}" -n "${namespace}" -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true
            kubectl delete nfsserver "${nfs_name}" -n "${namespace}" --force --grace-period=0 2>/dev/null || true
        done <<< "$nfs_servers"
    fi
    
    echo -e "${GREEN}Completed force deletion for SharedVolume: ${shared_volume_name}${NC}"
}

# Function to cleanup all stuck resources (nuclear option)
cleanup_all_stuck_resources() {
    echo -e "${RED}Starting comprehensive cleanup of ALL stuck resources...${NC}"
    
    # Force delete all pods stuck in Terminating state
    echo -e "${YELLOW}Cleaning up stuck pods...${NC}"
    local stuck_pods=$(kubectl get pods --all-namespaces --field-selector=status.phase=Terminating -o custom-columns="NAMESPACE:.metadata.namespace,NAME:.metadata.name" --no-headers)
    if [ -n "$stuck_pods" ]; then
        while IFS= read -r line; do
            local pod_namespace=$(echo "$line" | awk '{print $1}')
            local pod_name=$(echo "$line" | awk '{print $2}')
            echo "Force deleting stuck pod: $pod_name (namespace: $pod_namespace)"
            force_delete_pod "$pod_name" "$pod_namespace"
        done <<< "$stuck_pods"
    else
        echo "No stuck pods found"
    fi
    
    # Force delete all PVCs with finalizers
    echo -e "${YELLOW}Cleaning up stuck PVCs...${NC}"
    local stuck_pvcs=$(kubectl get pvc --all-namespaces -o jsonpath='{range .items[?(@.metadata.deletionTimestamp)]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}')
    if [ -n "$stuck_pvcs" ]; then
        while IFS= read -r line; do
            local pvc_namespace=$(echo "$line" | awk '{print $1}')
            local pvc_name=$(echo "$line" | awk '{print $2}')
            echo "Force deleting stuck PVC: $pvc_name (namespace: $pvc_namespace)"
            force_delete_pvc "$pvc_name" "$pvc_namespace"
        done <<< "$stuck_pvcs"
    else
        echo "No stuck PVCs found"
    fi
    
    # Force delete all PVs with finalizers
    echo -e "${YELLOW}Cleaning up stuck PVs...${NC}"
    local stuck_pvs=$(kubectl get pv -o jsonpath='{range .items[?(@.metadata.deletionTimestamp)]}{.metadata.name}{"\n"}{end}')
    if [ -n "$stuck_pvs" ]; then
        while IFS= read -r pv_name; do
            echo "Force deleting stuck PV: $pv_name"
            force_delete_pv "$pv_name"
        done <<< "$stuck_pvs"
    else
        echo "No stuck PVs found"
    fi
    
    echo -e "${GREEN}Comprehensive cleanup completed!${NC}"
}

# Main menu
case "${1:-}" in
    "pod")
        if [ $# -ne 3 ]; then
            echo -e "${RED}Usage: $0 pod <pod-name> <namespace>${NC}"
            exit 1
        fi
        force_delete_pod "$2" "$3"
        ;;
    "pvc")
        if [ $# -ne 3 ]; then
            echo -e "${RED}Usage: $0 pvc <pvc-name> <namespace>${NC}"
            exit 1
        fi
        force_delete_pvc "$2" "$3"
        ;;
    "pv")
        if [ $# -ne 2 ]; then
            echo -e "${RED}Usage: $0 pv <pv-name>${NC}"
            exit 1
        fi
        force_delete_pv "$2"
        ;;
    "sharedvolume")
        if [ $# -ne 3 ]; then
            echo -e "${RED}Usage: $0 sharedvolume <sharedvolume-name> <namespace>${NC}"
            exit 1
        fi
        force_delete_by_sharedvolume "$2" "$3"
        ;;
    "cleanup-all")
        echo -e "${RED}WARNING: This will force delete ALL stuck resources!${NC}"
        echo -e "${YELLOW}Are you sure? Type 'yes' to continue:${NC}"
        read -r confirmation
        if [ "$confirmation" = "yes" ]; then
            cleanup_all_stuck_resources
        else
            echo "Operation cancelled."
        fi
        ;;
    "status"|"show")
        show_stuck_resources
        ;;
    "")
        echo "Usage: $0 <command> [args...]"
        echo ""
        echo "Commands:"
        echo "  pod <name> <namespace>           Force delete a stuck pod"
        echo "  pvc <name> <namespace>           Force delete a stuck PVC"
        echo "  pv <name>                        Force delete a stuck PV"
        echo "  sharedvolume <name> <namespace>  Force delete all resources for a SharedVolume"
        echo "  cleanup-all                      Force delete ALL stuck resources (use with caution)"
        echo "  status                           Show stuck resources"
        echo ""
        echo "Examples:"
        echo "  $0 pod alpine-sample-pod-1 default"
        echo "  $0 pvc sharedvolume-with-sync-default default"
        echo "  $0 pv sharedvolume-with-sync-default"
        echo "  $0 sharedvolume sharedvolume-with-sync default"
        echo "  $0 status"
        ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        echo "Run '$0' without arguments to see usage information."
        exit 1
        ;;
esac
