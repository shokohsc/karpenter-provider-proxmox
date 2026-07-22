# PXE IPAM Cleanup Follow-Up Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix spurious IPAM release errors when deleting PXE-booted nodes.

**Architecture:** Annotate nodeClaims with the boot method during creation. In `instanceDelete`, check the annotation to skip IPAM release for PXE nodes (which never had IPs allocated via IPAM).

**Tech Stack:** Go, Kubernetes CRDs, Karpenter provider framework

## Context

The PXE boot feature (committed as `373cfeb..2f343ba`) skips IPAM allocation for PXE nodes during creation. But `instanceDelete` unconditionally calls `ReleaseIP` for all network interfaces on the deleted VM. PXE nodes get `ErrNoSubnetFound` (logged, not fatal) — noisy in production.

**Root cause:** `instanceDelete` has no way to know the node's boot method. It receives `nodeClaim` but not `nodeClass`.

**Fix:** Store boot method as an annotation on the nodeClaim during creation, read it during deletion.

## Global Constraints

- Use existing annotation pattern from `pkg/apis/v1alpha1/annotations.go`
- Follow existing `lo.Assign` pattern for annotation setting
- Test framework: `testing` stdlib + `github.com/stretchr/testify/assert`
- Existing test files in `pkg/providers/instance/` use table-driven tests

---

### Task 1: Add boot method annotation constant

**Files:**
- Modify: `pkg/apis/v1alpha1/annotations.go`

**Interfaces:**
- Produces: `AnnotationProxmoxBootMethod` constant

- [ ] **Step 1: Add the annotation constant**

In `pkg/apis/v1alpha1/annotations.go`, add after line 53 (the last annotation constant):

```go
	AnnotationProxmoxBootMethod = apis.Group + "/proxmoxboot-method"
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/apis/v1alpha1/annotations.go
git commit -m "feat: add boot method annotation constant"
```

---

### Task 2: Set boot method annotation on nodeClaim during creation

**Files:**
- Modify: `pkg/cloudprovider/cloudprovider.go:144-148`

**Interfaces:**
- Consumes: `nodeClass.Spec.BootMethod`, `AnnotationProxmoxBootMethod` constant
- Produces: nodeClaim has boot method annotation set after creation

- [ ] **Step 1: Set annotation in Create method**

In `pkg/cloudprovider/cloudprovider.go`, the annotation block at line 144 already sets several annotations. Add the boot method annotation to the same `lo.Assign` call:

Replace lines 144-148:
```go
	nc.Annotations = lo.Assign(nc.Annotations, map[string]string{
		v1alpha1.AnnotationProxmoxNodeClassHash:         nodeClass.Hash(),
		v1alpha1.AnnotationProxmoxNodeClassHashVersion:  v1alpha1.ProxmoxNodeClassHashVersion,
		v1alpha1.AnnotationProxmoxNodeInPlaceUpdateHash: nodeClass.InPlaceHash(),
	})
```

With:
```go
	nc.Annotations = lo.Assign(nc.Annotations, map[string]string{
		v1alpha1.AnnotationProxmoxNodeClassHash:         nodeClass.Hash(),
		v1alpha1.AnnotationProxmoxNodeClassHashVersion:  v1alpha1.ProxmoxNodeClassHashVersion,
		v1alpha1.AnnotationProxmoxNodeInPlaceUpdateHash: nodeClass.InPlaceHash(),
		v1alpha1.AnnotationProxmoxBootMethod:            nodeClass.Spec.BootMethod,
	})
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/cloudprovider/cloudprovider.go
git commit -m "feat: annotate nodeClaim with boot method during creation"
```

---

### Task 3: Skip IPAM release for PXE nodes in instanceDelete

**Files:**
- Modify: `pkg/providers/instance/instance_utils.go:254-262`

**Interfaces:**
- Consumes: `nodeClaim.Annotations[v1alpha1.AnnotationProxmoxBootMethod]`

- [ ] **Step 1: Guard IPAM release with boot method check**

In `pkg/providers/instance/instance_utils.go`, wrap the IPAM release block (lines 254-262) with a check for PXE boot method:

Replace lines 254-262:
```go
	networkValues := cloudinit.GetNetworkConfigFromVirtualMachineConfig(vm.VirtualMachineConfig, nil)
	for _, iface := range networkValues.Interfaces {
		for _, cidr := range iface.Address4 {
			err := p.nodeIpamProvider.ReleaseIP(cidr)
			if err != nil {
				log.Error(err, "Failed to release IP", "cidr", cidr)
			}
		}
	}
```

With:
```go
	// ponytail: PXE nodes never had IPs allocated via IPAM, skip release to avoid ErrNoSubnetFound noise.
	if nodeClaim.Annotations[v1alpha1.AnnotationProxmoxBootMethod] != v1alpha1.BootMethodPXE {
		networkValues := cloudinit.GetNetworkConfigFromVirtualMachineConfig(vm.VirtualMachineConfig, nil)
		for _, iface := range networkValues.Interfaces {
			for _, cidr := range iface.Address4 {
				err := p.nodeIpamProvider.ReleaseIP(cidr)
				if err != nil {
					log.Error(err, "Failed to release IP", "cidr", cidr)
				}
			}
		}
	}
```

Note: `v1alpha1` is already imported in this file (line 28).

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/providers/instance/instance_utils.go
git commit -m "feat: skip IPAM release for PXE boot nodes on deletion"
```

---

### Task 4: Add test for IPAM skip logic

**Files:**
- Create: `pkg/providers/instance/instance_utils_test.go`

**Interfaces:**
- Consumes: `v1alpha1.AnnotationProxmoxBootMethod`, `v1alpha1.BootMethodPXE`

- [ ] **Step 1: Write test for boot method annotation check**

Create `pkg/providers/instance/instance_utils_test.go`:

```go
package instance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/sergelogvinov/karpenter-provider-proxmox/pkg/apis/v1alpha1"
)

func TestNodeClaimBootMethod(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantPXE     bool
	}{
		{
			name:        "cloudInit default (empty annotation)",
			annotations: map[string]string{},
			wantPXE:     false,
		},
		{
			name:        "cloudInit explicit",
			annotations: map[string]string{v1alpha1.AnnotationProxmoxBootMethod: v1alpha1.BootMethodCloudInit},
			wantPXE:     false,
		},
		{
			name:        "pxe boot method",
			annotations: map[string]string{v1alpha1.AnnotationProxmoxBootMethod: v1alpha1.BootMethodPXE},
			wantPXE:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nc := &karpv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			isPXE := nc.Annotations[v1alpha1.AnnotationProxmoxBootMethod] == v1alpha1.BootMethodPXE
			assert.Equal(t, tt.wantPXE, isPXE)
		})
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./pkg/providers/instance/ -run TestNodeClaimBootMethod -v`
Expected: PASS (3/3 tests pass)

- [ ] **Step 3: Commit**

```bash
git add pkg/providers/instance/instance_utils_test.go
git commit -m "test: add boot method annotation check test"
```

---

## Summary

| What | Why |
|------|-----|
| `AnnotationProxmoxBootMethod` constant | Follow existing annotation pattern |
| Set annotation in `Create` | Pass boot method info to delete path |
| Guard IPAM release in `instanceDelete` | Skip spurious `ErrNoSubnetFound` for PXE nodes |
| Test for annotation check | Document expected behavior |

## What's NOT Changed

- **No Provider interface changes** — annotation check is internal
- **No changes to IPAM provider** — skip logic is in the caller
- **No changes to cloud-init or PXE creation path** — only delete path
