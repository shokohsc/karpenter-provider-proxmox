# Talos PXE Boot Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PXE network boot support so Talos Linux nodes can be provisioned without cloud-init metadata.

**Architecture:** Add a `bootMethod` field to `ProxmoxNodeClass` (`cloudInit` default, `pxe` optional). When `pxe` is set, the provider skips cloud-init ISO generation, cloud-init network regeneration, bootstrap token creation, and IPAM allocation. The VM boots from network (PXE), fetches its Talos machine config from the PXE/Talos API, and joins the cluster. PXE templates are referenced via existing `ProxmoxUnmanagedTemplate` — no new CRD needed.

**Tech Stack:** Go, Kubernetes CRDs (kubebuilder), Proxmox API (go-proxmox), Karpenter provider framework

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `pkg/apis/v1alpha1/nodeclass.go` | Modify | Add `BootMethod` field to `ProxmoxNodeClassSpec` |
| `pkg/providers/instance/instance_utils.go` | Modify | Skip cloud-init + network setup when PXE |
| `pkg/controllers/nodeclaim/lifecycle/instancetregistered.go` | Modify | Skip cloud-init detach when PXE |
| `pkg/controllers/nodeclass/status/metadataoptions.go` | Modify | Skip templatesRef validation when PXE |
| `pkg/cloudprovider/cloudprovider.go` | Modify | Skip bootstrap token in Create when PXE |
| `examples/talos-pxe/` | Create | Example NodeClass + UnmanagedTemplate for Talos PXE |

---

### Task 1: Add `BootMethod` field to ProxmoxNodeClass CRD

**Files:**
- Modify: `pkg/apis/v1alpha1/nodeclass.go:64-114`

**Interfaces:**
- Produces: `ProxmoxNodeClassSpec.BootMethod` (string, default `cloudInit`)

- [ ] **Step 1: Add the BootMethod constant and field**

In `pkg/apis/v1alpha1/nodeclass.go`, add constants after the existing placement strategy constants (line ~37):

```go
const (
	// BootMethodCloudInit uses cloud-init ISO for node bootstrap (default)
	BootMethodCloudInit = "cloudInit"
	// BootMethodPXE uses PXE network boot for node bootstrap
	BootMethodPXE = "pxe"
)
```

Then add the field to `ProxmoxNodeClassSpec` after `MetadataOptions` (after line 97):

```go
	// BootMethod defines how nodes are bootstrapped.
	// "cloudInit" (default) uses a cloud-init ISO attached to the VM.
	// "pxe" uses PXE network boot — the VM boots from network, no cloud-init is attached.
	// When "pxe", metadataOptions, templatesRef, and valuesRef are ignored.
	// +kubebuilder:default=cloudInit
	// +kubebuilder:validation:Enum:={cloudInit,pxe}
	// +optional
	BootMethod string `json:"bootMethod,omitempty"`
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/apis/v1alpha1/nodeclass.go
git commit -m "feat: add BootMethod field to ProxmoxNodeClass CRD"
```

---

### Task 2: Skip cloud-init and network setup in instance creation when PXE

**Files:**
- Modify: `pkg/providers/instance/instance_utils.go:46-215`

**Interfaces:**
- Consumes: `nodeClass.Spec.BootMethod`
- Produces: `instanceCreate()` skips cloud-init ISO and network setup when `bootMethod == "pxe"`

- [ ] **Step 1: Skip network setup and cloud-init for PXE boot**

In `pkg/providers/instance/instance_utils.go`, modify `instanceCreate()` to skip the network setup and cloud-init blocks when bootMethod is PXE.

Replace lines 146-174 (the network setup + firewall + cloud-init block) with:

```go
	if nodeClass.Spec.BootMethod != BootMethodPXE {
		err = p.instanceNetworkSetup(ctx, region, zone, newID)
		if err != nil {
			return nil, fmt.Errorf("failed to configure networking for vm %d: %v", newID, err)
		}
	}

	rules := make([]*proxmox.FirewallRule, len(nodeClass.Spec.SecurityGroups))
	for i, sg := range nodeClass.Spec.SecurityGroups {
		rules[i] = &proxmox.FirewallRule{
			Enable: 1,
			Pos:    i,
			Type:   "group",
			Action: sg.Name,
			Iface:  sg.Interface,
		}
	}

	if len(rules) > 0 {
		err = px.CreateVMFirewallRules(ctx, newID, zone, rules)
		if err != nil {
			return nil, fmt.Errorf("failed to create firewall rules for vm %d: %v", newID, err)
		}
	}

	if nodeClass.Spec.BootMethod != BootMethodPXE && nodeClass.Spec.MetadataOptions.Type == "cdrom" {
		err = p.attachCloudInitISO(ctx, nodeClaim, nodeClass, instanceTemplate, instanceType, region, zone, newID)
		if err != nil {
			return nil, fmt.Errorf("failed to attach cloud-init ISO to vm %d: %v", newID, err)
		}
	}
```

Also add the `BootMethodPXE` constant at the top of the file (or import it from v1alpha1):

```go
const BootMethodPXE = "pxe"
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/providers/instance/instance_utils.go
git commit -m "feat: skip cloud-init and network setup for PXE boot method"
```

---

### Task 3: Skip cloud-init detach for PXE nodes

**Files:**
- Modify: `pkg/controllers/nodeclaim/lifecycle/instancetregistered.go:45-82`

**Interfaces:**
- Consumes: `nodeClass.Spec.BootMethod`

- [ ] **Step 1: Skip cloud-init detach when PXE**

In `pkg/controllers/nodeclaim/lifecycle/instancetregistered.go`, modify the `Reconcile` method. The cloud-init detach block (lines 63-68) should also check bootMethod:

Replace lines 63-68:

```go
	if nodeClass.Spec.BootMethod != "pxe" && nodeClass.Spec.MetadataOptions.Type == "cdrom" {
		err = i.instanceProvider.DetachCloudInit(ctx, nodeClaim)
		if err != nil {
			return reconcile.Result{RequeueAfter: 5 * time.Second}, err
		}
	}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/controllers/nodeclaim/lifecycle/instancetregistered.go
git commit -m "feat: skip cloud-init detach for PXE boot nodes"
```

---

### Task 4: Skip metadata options validation when PXE

**Files:**
- Modify: `pkg/controllers/nodeclass/status/metadataoptions.go:41-72`

**Interfaces:**
- Consumes: `nodeClass.Spec.BootMethod`

- [ ] **Step 1: Early return when PXE boot method**

In `pkg/controllers/nodeclass/status/metadataoptions.go`, add a check at the start of `Reconcile`:

After line 41 (start of Reconcile function), add:

```go
	if nodeClass.Spec.BootMethod == "pxe" {
		nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionInstanceMetadataOptionsReady)
		return reconcile.Result{}, nil
	}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/controllers/nodeclass/status/metadataoptions.go
git commit -m "feat: skip metadata options validation for PXE boot method"
```

---

### Task 5: Skip bootstrap token creation for PXE nodes

**Files:**
- Modify: `pkg/cloudprovider/cloudprovider.go`

**Interfaces:**
- Consumes: `nodeClass.Spec.BootMethod`

- [ ] **Step 1: Find where bootstrap token is created in the Create flow**

The bootstrap token is created inside `attachCloudInitISO()` → `generateCloudInitVars()` at line 209 of `instance_cloudinit.go`:

```go
bootstrapToken, err := p.kubernetesBootstrapProvider.CreateToken(ctx, nodeClaim)
```

Since we already skip `attachCloudInitISO()` entirely when PXE (Task 2), the bootstrap token is never created. **No additional change needed here** — the token creation is already gated by the cloud-init flow.

- [ ] **Step 2: Verify with a quick check**

Confirm that `generateCloudInitVars` is only called from `attachCloudInitISO`, which is only called from `instanceCreate` inside the `BootMethod != PXE` guard. This is already the case after Task 2.

- [ ] **Step 3: Commit (no-op, verification only)**

No code change needed. Skip this task if executing.

---

### Task 6: Create Talos PXE boot example

**Files:**
- Create: `examples/talos-pxe/nodeclass.yaml`
- Create: `examples/talos-pxe/unmanaged-template.yaml`
- Create: `examples/talos-pxe/README.md`

**Interfaces:**
- Consumes: All changes from Tasks 1-4

- [ ] **Step 1: Create the example NodeClass**

Create `examples/talos-pxe/nodeclass.yaml`:

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: talos-pxe
spec:
  template:
    spec:
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]
        - key: karpenter.k8s.aws/instance-category
          operator: In
          values: ["general", "compute"]
      nodeClassRef:
        group: karpenter.proxmox.io
        kind: ProxmoxNodeClass
        name: talos-pxe
  limits:
    cpu: "100"
  disruption:
    consolidationPolicy: WhenUnderutilized
---
apiVersion: karpenter.proxmox.io/v1alpha1
kind: ProxmoxNodeClass
metadata:
  name: talos-pxe
spec:
  region: "pve-1"
  bootMethod: pxe
  instanceTemplateRef:
    kind: ProxmoxUnmanagedTemplate
    name: talos-pxe-template
  bootDevice:
    size: 50G
    storage: "local-lvm"
  tags:
    - "talos"
    - "pxe"
```

- [ ] **Step 2: Create the UnmanagedTemplate for PXE**

Create `examples/talos-pxe/unmanaged-template.yaml`:

```yaml
apiVersion: karpenter.proxmox.io/v1alpha1
kind: ProxmoxUnmanagedTemplate
metadata:
  name: talos-pxe-template
spec:
  region: "pve-1"
  templateName: "talos-pxe-base"
  tags:
    - "talos"
    - "pxe"
```

- [ ] **Step 3: Create README**

Create `examples/talos-pxe/README.md`:

```markdown
# Talos PXE Boot on Proxmox

This example provisions Talos Linux worker nodes via PXE network boot,
without cloud-init metadata.

## Prerequisites

1. A PXE/TFTP server serving the Talos kernel and initrd
2. A Proxmox VM template configured for PXE boot (network-first boot order, no cloud-init)
3. The template tagged with `talos` and `pxe`

## How It Works

When `bootMethod: pxe` is set on the ProxmoxNodeClass:

- The provider skips cloud-init ISO generation entirely
- No bootstrap token is created
- No IPAM allocation is performed (VM gets IP via DHCP from PXE server)
- The VM boots from network, loads Talos via PXE
- Talos fetches its machine config from the Talos API or a configured URL
- After Talos installs and joins the cluster, Karpenter marks the node as ready

## Setup

1. Create a PXE-ready Proxmox template:
   - Create a VM with a network device (virtio, bridged)
   - Set boot order: `net0` first
   - Optionally add a small disk for Talos installation
   - Convert to template
   - Tag with `talos` and `pxe`

2. Apply the resources:
   ```bash
   kubectl apply -f unmanaged-template.yaml
   kubectl apply -f nodeclass.yaml
   ```

3. The NodePool will now provision Talos nodes via PXE boot.
```

- [ ] **Step 4: Commit**

```bash
git add examples/talos-pxe/
git commit -m "docs: add Talos PXE boot example"
```

---

### Task 7: Regenerate CRD manifests

**Files:**
- Modify: `pkg/crds/` (generated)

- [ ] **Step 1: Run CRD generation**

Run: `make generate manifests` (or equivalent CRD generation command)

- [ ] **Step 2: Verify CRD includes bootMethod**

Check that the generated CRD YAML includes the new `bootMethod` field with enum validation.

- [ ] **Step 3: Commit**

```bash
git add pkg/crds/
git commit -m "chore: regenerate CRD manifests for bootMethod field"
```

---

## Summary of Changes

| What | Why |
|------|-----|
| `bootMethod` field on NodeClass | Controls whether cloud-init or PXE is used |
| Skip `instanceNetworkSetup()` for PXE | PXE VMs get network config from DHCP/PXE server |
| Skip `attachCloudInitISO()` for PXE | No cloud-init metadata needed for Talos PXE |
| Skip `DetachCloudInit()` for PXE | No cloud-init ISO to detach |
| Skip metadata options validation for PXE | No templatesRef needed for PXE |
| Example YAML | Reference implementation for users |

## What's NOT Changed

- **No new CRD** — PXE templates use existing `ProxmoxUnmanagedTemplate`
- **No Talos-specific Go code** — Talos is supported via PXE infrastructure, not provider code
- **No changes to the Provider interface** — all changes are internal to existing methods
- **No changes to drift detection** — template drift detection works the same way
- **No changes to in-place update** — tags, firewall, pool updates work the same way

## When to Add More

- If users need static IPs with PXE (not just DHCP), add IPAM support to the PXE path
- If users want the provider to manage PXE server configuration, that's a separate feature
- If Talos-specific machine config generation is needed (beyond what PXE provides), that would be a new `TalosConfig` CRD
