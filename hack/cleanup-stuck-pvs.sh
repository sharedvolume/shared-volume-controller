#!/bin/bash

# cleanup-stuck-pvs.sh
# This script helps clean up PVs that are stuck in Terminating state
# due to CSI finalizers when NFS services are already deleted

set -e

echo "🔍 Looking for PVs stuck in Terminating state..."

# Find PVs that are stuck in Terminating state
STUCK_PVS=$(kubectl get pv --no-headers | grep Terminating | awk '{print $1}')

if [ -z "$STUCK_PVS" ]; then
    echo "✅ No PVs stuck in Terminating state found."
    exit 0
fi

echo "⚠️  Found PVs stuck in Terminating state:"
echo "$STUCK_PVS"
echo ""

# Ask for confirmation
read -p "Do you want to force delete these PVs by removing their finalizers? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ Cancelled."
    exit 0
fi

echo "🧹 Removing finalizers from stuck PVs..."

# Remove finalizers from each stuck PV
for pv in $STUCK_PVS; do
    echo "  - Processing PV: $pv"
    
    # Check if PV still exists and is terminating
    if kubectl get pv "$pv" --no-headers 2>/dev/null | grep -q Terminating; then
        echo "    Removing finalizers from $pv..."
        if kubectl patch pv "$pv" -p '{"metadata":{"finalizers":null}}' --type=merge; then
            echo "    ✅ Successfully patched $pv"
        else
            echo "    ❌ Failed to patch $pv"
        fi
    else
        echo "    ⏭️  PV $pv is no longer terminating or doesn't exist"
    fi
done

echo ""
echo "🎉 Cleanup completed! Checking final state..."
kubectl get pv --no-headers | grep Terminating || echo "✅ No more PVs stuck in Terminating state."
